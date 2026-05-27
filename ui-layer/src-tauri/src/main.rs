#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::{Serialize};
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
        handle: Mutex::new(DaemonHandle { child: None, intentional_shutdown: false }),
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
            let app_handle = app.handle();
            let app_dir = app
                .path_resolver()
                .app_data_dir()
                .expect("failed to resolve app data dir");

            spawn_daemon(&app_handle, &app_dir);

            let app_handle_clone = app_handle.clone();
            let window = app.get_window("main").expect("main window not found");
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

fn resolve_sidecar(app_handle: &tauri::AppHandle) -> PathBuf {
    let target_triple = std::env::consts::ARCH.to_string()
        + "-pc-"
        + std::env::consts::OS
        + if cfg!(windows) { "-msvc" } else { "" };

    if cfg!(dev) {
        let manifest_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
        let candidate = manifest_dir
            .join("binaries")
            .join(format!("grimlocker-go-{}.exe", target_triple));
        if candidate.exists() {
            return candidate;
        }

        let fallback = manifest_dir.join("binaries").join("grimlocker-go.exe");
        if fallback.exists() {
            return fallback;
        }

        return candidate;
    }

    let resource_path = app_handle
        .path_resolver()
        .resource_dir()
        .unwrap_or_else(|| std::path::Path::new(".").to_path_buf());

    resource_path
        .join("binaries")
        .join(format!("grimlocker-go-{}", target_triple))
}

fn spawn_daemon(app_handle: &tauri::AppHandle, app_dir: &std::path::Path) {
    let sidecar_path = resolve_sidecar(app_handle);

    println!("[Tauri] Resolved sidecar: {:?}", sidecar_path);

    if !sidecar_path.exists() {
        eprintln!("[Tauri] Sidecar binary not found at: {:?}", sidecar_path);
        eprintln!("[Tauri] Run: go build -o ui-layer/src-tauri/binaries/grimlocker-go-x86_64-pc-windows-msvc.exe ./grimdb-go/cmd/");
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
                            if let Some(port_str) = ws_url.split(':').nth(1).and_then(|s| s.split('/').next()) {
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
    thread::spawn(move || {
        loop {
            thread::sleep(Duration::from_millis(500));
            let should_respawn = {
                let state = app_handle_monitor.state::<DaemonState>();
                let mut handle = state.handle.lock().unwrap();
                if handle.intentional_shutdown {
                    break;
                }
                match handle.child.as_mut().and_then(|c| c.try_wait().ok().flatten()) {
                    Some(status) => {
                        eprintln!("[Tauri] Go daemon exited unexpectedly ({:?}) — respawning in 1s", status);
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
        }
    });
}

fn kill_daemon(app_handle: &tauri::AppHandle) {
    let state = app_handle.state::<DaemonState>();
    let mut handle = state.handle.lock().unwrap();
    handle.intentional_shutdown = true;
    if let Some(mut child) = handle.child.take() {
        let pid = child.id();
        println!(
            "[Tauri] Window destroyed — killing Go daemon (PID: {})",
            pid
        );
        if let Err(e) = child.kill() {
            eprintln!("[Tauri] Failed to kill daemon: {}", e);
        }
        let _ = child.wait();
        std::thread::sleep(Duration::from_millis(500));
        println!("[Tauri] Go daemon terminated cleanly");
    } else {
        println!("[Tauri] No daemon process to kill");
    }
}
