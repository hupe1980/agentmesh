use serde::{Deserialize, Serialize};
use std::fs;

#[derive(Deserialize)]
struct Input {
    path: String,
}

#[derive(Serialize)]
struct Output {
    #[serde(skip_serializing_if = "Option::is_none")]
    result: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

static mut RESULT: Option<Vec<u8>> = None;

#[no_mangle]
pub extern "C" fn allocate(size: usize) -> *mut u8 {
    let mut buf = Vec::with_capacity(size);
    let ptr = buf.as_mut_ptr();
    std::mem::forget(buf);
    ptr
}

#[no_mangle]
pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) {
    unsafe {
        let input_slice = std::slice::from_raw_parts(input_ptr, input_len);
        let input_str = match std::str::from_utf8(input_slice) {
            Ok(s) => s,
            Err(e) => {
                let error_output = Output {
                    result: None,
                    error: Some(format!("invalid UTF-8: {}", e)),
                };
                RESULT = Some(serde_json::to_vec(&error_output).unwrap());
                return;
            }
        };

        let input: Input = match serde_json::from_str(input_str) {
            Ok(i) => i,
            Err(e) => {
                let error_output = Output {
                    result: None,
                    error: Some(format!("invalid JSON: {}", e)),
                };
                RESULT = Some(serde_json::to_vec(&error_output).unwrap());
                return;
            }
        };

        // Attempt to read files - this should be blocked by sandbox
        // Try multiple approaches to escape:
        // 1. Direct file read
        let output = match fs::read_to_string(&input.path) {
            Ok(content) => Output {
                result: Some(format!("Successfully read {} bytes from {}", content.len(), input.path)),
                error: None,
            },
            Err(e) => {
                // 2. Try reading directory
                match fs::read_dir(&input.path) {
                    Ok(entries) => {
                        let count = entries.count();
                        Output {
                            result: Some(format!("Successfully listed {} entries in {}", count, input.path)),
                            error: None,
                        }
                    },
                    Err(e2) => {
                        // 3. Try getting metadata
                        match fs::metadata(&input.path) {
                            Ok(meta) => Output {
                                result: Some(format!("Successfully accessed metadata for {}: {} bytes", input.path, meta.len())),
                                error: None,
                            },
                            Err(e3) => Output {
                                result: None,
                                error: Some(format!(
                                    "All filesystem access attempts failed - read: {}, readdir: {}, metadata: {}",
                                    e, e2, e3
                                )),
                            },
                        }
                    },
                }
            },
        };

        RESULT = Some(serde_json::to_vec(&output).unwrap());
    }
}

#[no_mangle]
pub extern "C" fn get_result_ptr() -> *const u8 {
    unsafe {
        match &RESULT {
            Some(vec) => vec.as_ptr(),
            None => std::ptr::null(),
        }
    }
}

#[no_mangle]
pub extern "C" fn get_result_len() -> usize {
    unsafe {
        match &RESULT {
            Some(vec) => vec.len(),
            None => 0,
        }
    }
}
