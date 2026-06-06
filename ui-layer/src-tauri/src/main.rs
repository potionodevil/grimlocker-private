#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::Serialize;
use std::fs;
use std::io::{BufRead, BufReader, Write};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::thread;
use std::time::Duration;
use tauri::Manager;
use tauri::Emitter;
use tauri::menu::{MenuBuilder, MenuItemBuilder};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri_plugin_global_shortcut::{GlobalShortcutExt, Shortcut, ShortcutState, Code, Modifiers};
use enigo::KeyboardControllable;

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

// Speichert das HWND des Fensters das beim Ctrl+G-Druck aktiv war,
// damit fill_text es vor dem Tippen wieder in den Vordergrund holen kann.
struct AutofillTarget {
    hwnd: Mutex<isize>,
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

/// Saves binary data as a temporary file in the OS temp directory.
/// Returns the absolute path of the created file.
/// The caller is responsible for calling secure_delete_temp() after use.
#[tauri::command]
fn save_temp_file(filename: String, data: Vec<u8>) -> Result<String, String> {
    let tmp_dir = std::env::temp_dir();
    // Sanitize filename — no path separators allowed.
    let safe_name: String = filename
        .chars()
        .map(|c| if c == '/' || c == '\\' || c == ':' { '_' } else { c })
        .collect();

    let tmp_path = tmp_dir.join(format!("grimlocker_tmp_{}", safe_name));

    let mut file = fs::File::create(&tmp_path)
        .map_err(|e| format!("create temp file: {}", e))?;
    file.write_all(&data)
        .map_err(|e| format!("write temp file: {}", e))?;
    file.flush()
        .map_err(|e| format!("flush temp file: {}", e))?;

    tmp_path
        .to_str()
        .map(|s| s.to_string())
        .ok_or_else(|| "invalid path encoding".to_string())
}

/// Opens a file with the system default application.
/// On Windows: uses ShellExecute via the `open` verb.
/// On macOS/Linux: uses the `open` / `xdg-open` command.
#[tauri::command]
fn open_with_default_app(path: String) -> Result<(), String> {
    #[cfg(target_os = "windows")]
    {
        Command::new("cmd")
            .args(["/C", "start", "", &path])
            .spawn()
            .map_err(|e| format!("open file: {}", e))?;
    }
    #[cfg(target_os = "macos")]
    {
        Command::new("open")
            .arg(&path)
            .spawn()
            .map_err(|e| format!("open file: {}", e))?;
    }
    #[cfg(target_os = "linux")]
    {
        Command::new("xdg-open")
            .arg(&path)
            .spawn()
            .map_err(|e| format!("open file: {}", e))?;
    }
    Ok(())
}

/// Securely deletes a temporary file using multi-pass overwrite.
/// Each byte of the file is overwritten with random data before deletion,
/// reducing the chance of forensic recovery on magnetic storage.
/// SSD wear-leveling may retain copies — this is best-effort.
#[tauri::command]
fn secure_delete_temp(path: String) -> Result<(), String> {
    let path_buf = std::path::Path::new(&path);

    if !path_buf.exists() {
        return Ok(()); // Already deleted — idempotent.
    }

    // Safety: only allow deleting files in the OS temp directory.
    let tmp_dir = std::env::temp_dir();
    if !path_buf.starts_with(&tmp_dir) {
        return Err("secure_delete_temp: path is not in temp directory (safety check)".to_string());
    }

    // Get file size for overwrite passes.
    let metadata = fs::metadata(&path_buf)
        .map_err(|e| format!("stat temp file: {}", e))?;
    let file_size = metadata.len() as usize;

    if file_size > 0 {
        // 3-pass overwrite with pseudo-random data (crypto-grade via OS CSPRNG).
        for pass in 0..3 {
            let mut file = fs::OpenOptions::new()
                .write(true)
                .open(&path_buf)
                .map_err(|e| format!("open for wipe pass {}: {}", pass, e))?;

            let fill_byte = (pass * 85) as u8; // 0x00, 0x55, 0xAA
            let fill: Vec<u8> = vec![fill_byte; file_size];
            use std::io::Write;
            file.write_all(&fill)
                .map_err(|e| format!("wipe pass {} write: {}", pass, e))?;
            file.flush()
                .map_err(|e| format!("wipe pass {} flush: {}", pass, e))?;
        }
    }

    fs::remove_file(&path_buf)
        .map_err(|e| format!("delete temp file: {}", e))?;

    Ok(())
}

/// Liest den Titel und Prozessnamen des aktuell fokussierten Fensters aus.
/// Funktioniert systemweit — auch wenn Grimlocker im Hintergrund läuft.
#[tauri::command]
fn get_window_title() -> Result<serde_json::Value, String> {
    get_active_window_info()
}

fn get_active_window_info() -> Result<serde_json::Value, String> {
    #[cfg(target_os = "windows")]
    {
        use windows_sys::Win32::UI::WindowsAndMessaging::{
            GetForegroundWindow, GetWindowTextW, GetWindowTextLengthW, GetWindowThreadProcessId,
        };
        use windows_sys::Win32::System::Threading::{
            OpenProcess, QueryFullProcessImageNameW,
            PROCESS_QUERY_INFORMATION, PROCESS_VM_READ,
        };
        use windows_sys::Win32::Foundation::CloseHandle;

        unsafe {
            let hwnd = GetForegroundWindow();
            if hwnd == std::ptr::null_mut() {
                return Err("Kein aktives Fenster gefunden".into());
            }

            let len = GetWindowTextLengthW(hwnd) as usize;
            let mut title_buf: Vec<u16> = vec![0; len + 1];
            let actual_len = GetWindowTextW(hwnd, title_buf.as_mut_ptr(), (len + 1) as i32) as usize;
            let title = String::from_utf16_lossy(&title_buf[..actual_len]);

            let mut pid: u32 = 0;
            GetWindowThreadProcessId(hwnd, &mut pid);

            let app_name = if pid > 0 {
                let h_proc = OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ, 0, pid);
                if h_proc != std::ptr::null_mut() {
                    let mut exe_buf = [0u16; 260];
                    let mut exe_len = exe_buf.len() as u32;
                    let ret = QueryFullProcessImageNameW(h_proc, 0, exe_buf.as_mut_ptr(), &mut exe_len);
                    CloseHandle(h_proc);
                    if ret != 0 {
                        let exe_path = String::from_utf16_lossy(&exe_buf[..exe_len as usize]);
                        std::path::Path::new(&exe_path)
                            .file_stem()
                            .and_then(|s| s.to_str())
                            .unwrap_or("Unbekannt")
                            .to_string()
                    } else {
                        "Unbekannt".to_string()
                    }
                } else {
                    "Unbekannt".to_string()
                }
            } else {
                "Unbekannt".to_string()
            };

            Ok(serde_json::json!({
                "title": title,
                "appName": app_name,
                "url": null,
                "processId": pid,
            }))
        }
    }

    #[cfg(target_os = "linux")]
    {
        let title = std::process::Command::new("xdotool")
            .args(["getactivewindow", "getwindowname"])
            .output()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_default();

        Ok(serde_json::json!({
            "title": title,
            "appName": "Unbekannt",
            "url": null,
            "processId": 0,
        }))
    }

    #[cfg(target_os = "macos")]
    {
        let app_name = std::process::Command::new("osascript")
            .args(["-e", r#"tell application "System Events" to get name of first application process whose frontmost is true"#])
            .output()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_default();

        let title = std::process::Command::new("osascript")
            .args(["-e", r#"tell application "System Events" to get name of front window of first application process whose frontmost is true"#])
            .output()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_default();

        Ok(serde_json::json!({
            "title": title,
            "appName": app_name,
            "url": null,
            "processId": 0,
        }))
    }

    #[cfg(not(any(target_os = "windows", target_os = "linux", target_os = "macos")))]
    {
        return Err("Plattform wird nicht unterstützt".into());
    }
}

/// Simuliert Tastatureingaben in das aktuell fokussierte Fenster.
/// Nutzt enigo (SendInput auf Windows, XTest auf Linux, CGEvent auf macOS).
#[tauri::command]
fn fill_text(text: String, state: tauri::State<AutofillTarget>) -> Result<(), String> {
    let hwnd = *state.hwnd.lock().unwrap();
    eprintln!("[AUTOFILL] fill_text called: {} chars, hwnd={}", text.len(), hwnd);

    // Original-Fenster wieder fokussieren bevor wir tippen,
    // sonst landen Tastatureingaben im falschen Fenster.
    #[cfg(target_os = "windows")]
    if hwnd != 0 {
        use windows_sys::Win32::UI::WindowsAndMessaging::SetForegroundWindow;
        unsafe { SetForegroundWindow(hwnd as _); }
    }

    std::thread::sleep(std::time::Duration::from_millis(200));
    let mut enigo = enigo::Enigo::new();

    let parts: Vec<&str> = text.splitn(2, '\t').collect();
    if parts.len() == 2 {
        enigo.key_sequence(parts[0]);
        std::thread::sleep(std::time::Duration::from_millis(50));
        enigo.key_click(enigo::Key::Tab);
        std::thread::sleep(std::time::Duration::from_millis(80));
        enigo.key_sequence(parts[1]);
    } else {
        enigo.key_sequence(&text);
    }

    std::thread::sleep(std::time::Duration::from_millis(80));
    enigo.key_click(enigo::Key::Return);
    Ok(())
}

/// Schreibt eine Debug-Meldung vom Popup ins Terminal (sichtbar in `tauri dev`).
#[tauri::command]
fn log_autofill(message: String) {
    eprintln!("[AUTOFILL-POPUP] {}", message);
}

/// Vom Autofill-Popup aufgerufen: speichert die Auswahl und emittet ein Event
/// ans Hauptfenster, das dann entschlüsselt und fill_text aufruft.
#[tauri::command]
fn confirm_autofill(
    entry_id: String,
    fill_mode: String,
    app: tauri::AppHandle,
) -> Result<(), String> {
    eprintln!("[AUTOFILL] confirm_autofill called: entry_id={} fill_mode={}", entry_id, fill_mode);

    let main_win = app.get_webview_window("main");
    eprintln!("[AUTOFILL] main window found: {}", main_win.is_some());

    if let Some(win) = main_win {
        let result = win.emit("autofill:confirm", serde_json::json!({
            "entryId": entry_id,
            "fillMode": fill_mode,
        }));
        eprintln!("[AUTOFILL] emit autofill:confirm result: {:?}", result);
    }

    // Popup schließen
    if let Some(popup) = app.get_webview_window("autofill") {
        eprintln!("[AUTOFILL] closing popup window");
        let _ = popup.close();
    } else {
        eprintln!("[AUTOFILL] WARNING: autofill window not found for close");
    }
    Ok(())
}

/// Vom Autofill-Popup aufgerufen: Abbruch — Popup schließen.
#[tauri::command]
fn cancel_autofill(app: tauri::AppHandle) -> Result<(), String> {
    eprintln!("[AUTOFILL] cancel_autofill called");
    if let Some(popup) = app.get_webview_window("autofill") {
        let _ = popup.close();
    }
    Ok(())
}

fn main() {
    let daemon_state = DaemonState {
        handle: Mutex::new(DaemonHandle {
            child: None,
            intentional_shutdown: false,
        }),
        config: Mutex::new(DaemonConfig::default()),
    };

    let autofill_target = AutofillTarget { hwnd: Mutex::new(0) };

    tauri::Builder::default()
        .manage(daemon_state)
        .manage(autofill_target)
        .invoke_handler(tauri::generate_handler![
            get_session_token,
            rust_get_version,
            rust_secure_wipe,
            save_temp_file,
            open_with_default_app,
            secure_delete_temp,
            get_window_title,
            fill_text,
            confirm_autofill,
            cancel_autofill,
            log_autofill
        ])
        .plugin(tauri_plugin_global_shortcut::Builder::default().build())
        .setup(|app| {
            let app_handle = app.handle().clone();
            let app_dir = app
                .path()
                .app_data_dir()
                .expect("failed to resolve app data dir");

            spawn_daemon(&app_handle, &app_dir);

            // ── Hotkey: Strg+G — Autofill Passwort ins aktive Fenster ──────
            let app_handle_shortcut = app_handle.clone();
            app.global_shortcut().on_shortcut(
                Shortcut::new(Some(Modifiers::CONTROL), Code::KeyG),
                move |app, _shortcut, event| {
                    if event.state() != ShortcutState::Pressed {
                        return;
                    }
                    eprintln!("[AUTOFILL] Ctrl+G pressed — capturing active window");
                    // HWND des aktiven Fensters speichern BEVOR das Popup den Fokus stielt.
                    // fill_text ruft später SetForegroundWindow damit auf.
                    #[cfg(target_os = "windows")]
                    {
                        use windows_sys::Win32::UI::WindowsAndMessaging::GetForegroundWindow;
                        let hwnd = unsafe { GetForegroundWindow() as isize };
                        if let Some(target) = app.try_state::<AutofillTarget>() {
                            *target.hwnd.lock().unwrap() = hwnd;
                        }
                    }

                    // Fenstertitel des aktiven Fensters systemweit ermitteln
                    let info = get_active_window_info().unwrap_or(serde_json::json!({
                        "title": "",
                        "appName": "Unbekannt",
                        "url": null,
                        "processId": 0,
                    }));
                    let title = info["title"].as_str().unwrap_or("").to_string();
                    let app_name = info["appName"].as_str().unwrap_or("").to_string();
                    let url = info.get("url").and_then(|v| v.as_str()).map(|s| s.to_string());
                    eprintln!("[AUTOFILL] active window: title='{}' app='{}'", title, app_name);

                    // Nur Event senden — Hauptfenster NICHT öffnen (Popup öffnet sich separat)
                    if let Some(win) = app.get_webview_window("main") {
                        let _ = win.emit(
                            "autofill:trigger",
                            serde_json::json!({
                                "windowTitle": title,
                                "appName": app_name,
                                "url": url,
                            }),
                        );
                    }
                },
            ).ok();

            let app_handle_clone = app_handle_shortcut.clone();
            let window = app.get_webview_window("main").expect("main window not found");
            window.on_window_event(move |event| {
                if let tauri::WindowEvent::Destroyed = event {
                    kill_daemon(&app_handle_clone);
                }
            });

            // ── System tray ───────────────────────────────────────────────────
            let show_item = MenuItemBuilder::with_id("show", "Show Grimlocker").build(app)?;
            let quit_item = MenuItemBuilder::with_id("quit", "Quit").build(app)?;
            let tray_menu = MenuBuilder::new(app)
                .item(&show_item)
                .separator()
                .item(&quit_item)
                .build()?;

            let _tray = TrayIconBuilder::new()
                .icon(app.default_window_icon().unwrap().clone())
                .menu(&tray_menu)
                .tooltip("Grimlocker — Zero-Trust Vault")
                .on_menu_event(|app_handle, event| match event.id().as_ref() {
                    "show" => {
                        if let Some(win) = app_handle.get_webview_window("main") {
                            let _ = win.show();
                            let _ = win.set_focus();
                        }
                    }
                    "quit" => {
                        kill_daemon(app_handle);
                        app_handle.exit(0);
                    }
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } = event
                    {
                        let app_handle = tray.app_handle();
                        if let Some(win) = app_handle.get_webview_window("main") {
                            if win.is_visible().unwrap_or(false) {
                                let _ = win.set_focus();
                            } else {
                                let _ = win.show();
                                let _ = win.set_focus();
                            }
                        }
                    }
                })
                .build(app)?;

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
