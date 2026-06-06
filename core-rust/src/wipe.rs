//! Sicheres Löschen von Dateien — 7-Pass-Überschreiben + Truncate + Unlink.
//!
//! # Warum?
//! Ein `rm` oder `Remove-Item` löscht nur die Directory-Entry,
//! die Daten bleiben auf der Platte lesbar (Forensik).
//! Dieses Modul überschreibt die Datei mehrfach mit Zufallsdaten,
//! bevor sie gelöscht wird.
//!
//! # Threat Model
//! - Recovery-Tools (PhotoRec, TestDisk) können gelöschte Dateien
//!   wiederherstellen → 7 Passes mit CSPRNG-Daten machen das unmöglich.
//! - SSD-Controller haben internen Cache → `sync_all()` nach jedem Pass
//!   zwingt die Daten auf das Medium.
//! - Datei-Metadaten (Name, Timestamps) bleiben erhalten → das ist
//!   akzeptiert; für vollständige Vernichtung müsste man den ganzen
//!   Speicherblock überschreiben (jenseits von Rusts Möglichkeiten).
//!
//! # Design Trade-offs
//! - 7 Passes = guter Kompromiss zwischen Sicherheit und Geschwindigkeit
//!   (NIST SP 800-88 empfiehlt 1 Pass mit Zufallsdaten, wir gehen
//!   auf Nummer sicher)
//! - 64KB Buffer → reduziert Syscall-Overhead bei großen Dateien
//! - `secure_wipe_file_contents` überschreibt nur den Inhalt, löscht
//!   die Datei nicht (für laufende Dateihandles gedacht)

use crate::Error;
use rand::RngCore;
use std::fs::OpenOptions;
use std::io::{Seek, SeekFrom, Write};
use std::path::Path;

const WIPE_PASSES: usize = 7;

pub fn secure_wipe<P: AsRef<Path>>(path: P) -> Result<(), Error> {
    let path = path.as_ref();

    let metadata =
        std::fs::metadata(path).map_err(|e| Error::Wipe(format!("cannot stat file: {}", e)))?;

    let file_size = metadata.len();

    if file_size == 0 {
        std::fs::remove_file(path).map_err(|e| Error::Wipe(format!("remove empty file: {}", e)))?;
        return Ok(());
    }

    let mut file = OpenOptions::new()
        .write(true)
        .read(false)
        .create(false)
        .open(path)
        .map_err(|e| Error::Wipe(format!("open for wipe: {}", e)))?;

    let mut rng = rand::thread_rng();
    let mut buffer = vec![0u8; 65536];

    for pass in 0..WIPE_PASSES {
        file.seek(SeekFrom::Start(0))
            .map_err(|e| Error::Wipe(format!("seek pass {}: {}", pass, e)))?;

        let mut remaining = file_size;

        while remaining > 0 {
            let chunk_size = std::cmp::min(buffer.len() as u64, remaining) as usize;

            rng.fill_bytes(&mut buffer[..chunk_size]);

            file.write_all(&buffer[..chunk_size])
                .map_err(|e| Error::Wipe(format!("write pass {}, offset: {}", pass, e)))?;

            remaining -= chunk_size as u64;
        }

        file.sync_all()
            .map_err(|e| Error::Wipe(format!("sync pass {}: {}", pass, e)))?;
    }

    file.set_len(0)
        .map_err(|e| Error::Wipe(format!("truncate: {}", e)))?;

    file.sync_all()
        .map_err(|e| Error::Wipe(format!("sync after truncate: {}", e)))?;

    drop(file);

    std::fs::remove_file(path).map_err(|e| Error::Wipe(format!("unlink: {}", e)))?;

    Ok(())
}

pub fn secure_wipe_file_contents<P: AsRef<Path>>(path: P) -> Result<(), Error> {
    let path = path.as_ref();

    let metadata =
        std::fs::metadata(path).map_err(|e| Error::Wipe(format!("cannot stat file: {}", e)))?;

    let file_size = metadata.len();

    let mut file = OpenOptions::new()
        .write(true)
        .read(false)
        .open(path)
        .map_err(|e| Error::Wipe(format!("open for content wipe: {}", e)))?;

    let mut rng = rand::thread_rng();
    let mut buffer = vec![0u8; 65536];

    file.seek(SeekFrom::Start(0))
        .map_err(|e| Error::Wipe(format!("seek: {}", e)))?;

    let mut remaining = file_size;
    while remaining > 0 {
        let chunk_size = std::cmp::min(buffer.len() as u64, remaining) as usize;
        rng.fill_bytes(&mut buffer[..chunk_size]);
        file.write_all(&buffer[..chunk_size])
            .map_err(|e| Error::Wipe(format!("write: {}", e)))?;
        remaining -= chunk_size as u64;
    }

    file.sync_all()
        .map_err(|e| Error::Wipe(format!("sync: {}", e)))?;

    file.set_len(0)
        .map_err(|e| Error::Wipe(format!("truncate: {}", e)))?;

    file.sync_all()
        .map_err(|e| Error::Wipe(format!("final sync: {}", e)))?;

    Ok(())
}
