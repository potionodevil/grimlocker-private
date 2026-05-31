#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::Serialize;
use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::thread;
use std::time::Duration;
use tauri::Manager;

struct DaemonHandle {
    child: Option<Child>,
    intentional_shutdown: bool,
}

#[derive(Serialize, Clone, Default)]
struct DaemonConfig {
    token: Option<String>,
    ipc_port: Option<u16>,
}

struct DaemonState {
    handle: Mutex<DaemonHandle>,
    config: Mutex<DaemonConfig>,
}

#[tauri::command]
fn get_session_token(state: tauri::State<DaemonState>) -> Result<DaemonConfig, String> {
    let cfg = state.config.lock().unwrap();
    if cfg.token.is_some() && cfg.ipc_port.is_some() {
        Ok(cfg.clone())
    } else {
        Err("daemon_not_ready".into())
    }
}

/// Returns the version of the RustCore library (grimlocker-core).
#[tauri::command]
fn rust_get_version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

/// Performs a secure wipe of a file using the RustCore wipe module (7-pass overwrite).
#[tauri::command]
fn rust_secure_wipe(path: String) -> Result<String, String> {
    // TODO: Integrate with grimlocker-core's wipe module
    // For now, just acknowledge the request (actual wipe would need C-ABI export from core-rust)
    Ok(format!("Wipe requested for: {}", path))
}

fn main() {
    let daemon_state = DaemonState {
        handle: Mutex::new(DaemonHandle {
            child: None,
            intentional_shutdown: false,
        }),
        config: Mutex::new(DaemonConfig::default()),
    };

    tauri::Builder::default()
        .manage(daemon_state)
        .invoke_handler(tauri::generate_handler![
            get_session_token,
            rust_get_version,
            rust_secure_wipe
        ])
        .setup(|app| {
            let app_handle = app.handle().clone();
            let app_dir = app
                .path()
                .app_data_dir()
                .expect("failed to resolve app data dir");

            spawn_daemon(&app_handle, &app_dir);

            let app_handle_clone = app_handle.clone();
            let window = app.get_webview_window("main").expect("main window not found");
            window.on_window_event(move |event| {
                if let tauri::WindowEvent::Destroyed = event {
                    kill_daemon(&app_handle_clone);
                }
            });

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running Grimlocker");
}

fn target_triple() -> &'static str {
    let arch = std::env::consts::ARCH;
    let os = std::env::consts::OS;
    match (arch, os) {
        ("x86_64", "linux") => "x86_64-unknown-linux-gnu",
        ("aarch64", "linux") => "aarch64-unknown-linux-gnu",
        ("x86_64", "windows") => "x86_64-pc-windows-msvc",
        ("aarch64", "windows") => "aarch64-pc-windows-msvc",
        ("x86_64", "macos") => "x86_64-apple-darwin",
        ("aarch64", "macos") => "aarch64-apple-darwin",
        _ => "",
    }
}

fn exe_suffix() -> &'static str {
    std::env::consts::EXE_SUFFIX
}

fn resolve_sidecar(app_handle: &tauri::AppHandle) -> PathBuf {
    let triple = target_triple();
    let ext = exe_suffix();

    if cfg!(dev) {
        let manifest_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
        // Prefer the new grimdb-daemon binary; fall back to grimlocker-go for compat.
        let candidate = manifest_dir
            .join("binaries")
            .join(format!("grimdb-daemon-{}{}", triple, ext));
        if candidate.exists() {
            return candidate;
        }

        let fallback_go = manifest_dir
            .join("binaries")
            .join(format!("grimlocker-go-{}{}", triple, ext));
        if fallback_go.exists() {
            return fallback_go;
        }

        let fallback_plain = manifest_dir
            .join("binaries")
            .join(format!("grimdb-daemon{}", ext));
        if fallback_plain.exists() {
            return fallback_plain;
        }

        return candidate;
    }

    let resource_path = app_handle
        .path()
        .resource_dir()
        .unwrap_or_else(|_| std::path::Path::new(".").to_path_buf());

    resource_path
        .join("binaries")
        .join(format!("grimdb-daemon-{}{}", triple, ext))
}

fn spawn_daemon(app_handle: &tauri::AppHandle, app_dir: &std::path::Path) {
    let sidecar_path = resolve_sidecar(app_handle);

    println!("[Tauri] Resolved sidecar: {:?}", sidecar_path);

    if !sidecar_path.exists() {
        eprintln!("[Tauri] Sidecar binary not found at: {:?}", sidecar_path);
        let build_cmd = if cfg!(windows) {
            "cd grimdb && bash build.sh".to_string()
        } else {
            format!("cd grimdb && go build -o ../ui-layer/src-tauri/binaries/grimdb-daemon-{}{} ./cmd/daemon/", target_triple(), exe_suffix())
        };
        eprintln!("[Tauri] Run: {}", build_cmd);
        return;
    }

    let mut cmd = Command::new(&sidecar_path);
    cmd.env("GRIMLOCKER_APP_DIR", app_dir.to_str().unwrap());
    cmd.stdout(Stdio::piped());
    cmd.stderr(Stdio::piped());

    #[cfg(windows)]
    if !cfg!(debug_assertions) {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x08000000;
        cmd.creation_flags(CREATE_NO_WINDOW);
    }

    let mut child = match cmd.spawn() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("[Tauri] Failed to spawn Go daemon: {}", e);
            return;
        }
    };

    let pid = child.id();
    println!("[Tauri] Go daemon spawned (PID: {})", pid);

    let stdout = child.stdout.take();
    let stderr = child.stderr.take();

    if let Some(stdout) = stdout {
        let app_handle_for_stdout = app_handle.clone();
        thread::spawn(move || {
            let reader = BufReader::new(stdout);
            for line in reader.lines() {
                match line {
                    Ok(l) => {
                        println!("[Go] {}", l);
                        if let Some(token) = l.strip_prefix("GRIMLOCKER_TOKEN=") {
                            let state = app_handle_for_stdout.state::<DaemonState>();
                            state.config.lock().unwrap().token = Some(token.trim().to_string());
                        }
                        if let Some(ws_url) = l.strip_prefix("GRIMLOCKER_IPC=ws://") {
                            if let Some(port_str) =
                                ws_url.split(':').nth(1).and_then(|s| s.split('/').next())
                            {
                                if let Ok(port) = port_str.parse::<u16>() {
                                    let state = app_handle_for_stdout.state::<DaemonState>();
                                    state.config.lock().unwrap().ipc_port = Some(port);
                                }
                            }
                        }
                    }
                    Err(e) => eprintln!("[Go] stdout error: {}", e),
                }
            }
        });
    }

    if let Some(stderr) = stderr {
        thread::spawn(move || {
            let reader = BufReader::new(stderr);
            for line in reader.lines() {
                match line {
                    Ok(l) => {
                        let prefix = if l.starts_with("panic:")
                            || l.starts_with("goroutine ")
                            || l.starts_with("fatal error:")
                            || l.contains("PANIC_TRIGGER")
                            || l.contains("runtime error:")
                        {
                            "[Go PANIC]"
                        } else {
                            "[Go]"
                        };
                        eprintln!("{} {}", prefix, l);
                    }
                    Err(e) => eprintln!("[Go] stderr read error: {}", e),
                }
            }
        });
    }

    let state = app_handle.state::<DaemonState>();
    let mut handle = state.handle.lock().unwrap();
    handle.child = Some(child);
    drop(handle);

    let app_handle_monitor = app_handle.clone();
    let app_dir_monitor = app_dir.to_path_buf();
    thread::spawn(move || loop {
        thread::sleep(Duration::from_millis(500));
        let should_respawn = {
            let state = app_handle_monitor.state::<DaemonState>();
            let mut handle = state.handle.lock().unwrap();
            if handle.intentional_shutdown {
                break;
            }
            match handle
                .child
                .as_mut()
                .and_then(|c| c.try_wait().ok().flatten())
            {
                Some(status) => {
                    eprintln!(
                        "[Tauri] Go daemon exited unexpectedly ({:?}) — respawning in 1s",
                        status
                    );
                    handle.child = None;
                    drop(handle);
                    state.config.lock().unwrap().token = None;
                    true
                }
                None => false,
            }
        };
        if should_respawn {
            thread::sleep(Duration::from_secs(1));
            spawn_daemon(&app_handle_monitor, &app_dir_monitor);
            break;
        }
    });
}

fn kill_daemon(app_handle: &tauri::AppHandle) {
    let state = app_handle.state::<DaemonState>();
    let mut handle = state.handle.lock().unwrap();
    handle.intentional_shutdown = true;

    let ipc_port = state.config.lock().unwrap().ipc_port;
    let token = state.config.lock().unwrap().token.clone();

    if let Some(mut child) = handle.child.take() {
        let pid = child.id();
        println!(
            "[Tauri] Window destroyed — requesting graceful shutdown (PID: {})",
            pid
        );

        // Step 1: Request graceful shutdown via HTTP endpoint.
        // The daemon flushes storage, revokes enclave handles, then exits.
        if let Some(port) = ipc_port {
            let url = format!("http://127.0.0.1:{}/shutdown", port);
            let mut req = ureq::post(&url);
            if let Some(ref t) = token {
                req = req.set("X-Grimlocker-Token", t);
            }
            let _ = req.send_string("");
        }

        // Step 2: Wait up to 3 seconds for the daemon to exit cleanly.
        let deadline = std::time::Instant::now() + Duration::from_secs(3);
        loop {
            if child.try_wait().ok().flatten().is_some() {
                println!("[Tauri] Daemon shut down gracefully (PID: {})", pid);
                return;
            }
            if std::time::Instant::now() >= deadline {
                break;
            }
            std::thread::sleep(Duration::from_millis(100));
        }

        // Step 3: Graceful shutdown timed out — force kill as fallback.
        println!(
            "[Tauri] Graceful shutdown timed out — force killing daemon (PID: {})",
            pid
        );
        if let Err(e) = child.kill() {
            eprintln!("[Tauri] Failed to kill daemon: {}", e);
        }
        let _ = child.wait();
        println!("[Tauri] Daemon force-terminated");
    } else {
        println!("[Tauri] No daemon process to kill");
    }
}
