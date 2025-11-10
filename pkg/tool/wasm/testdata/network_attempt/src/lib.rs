use serde::{Deserialize, Serialize};
use std::net::{TcpStream, ToSocketAddrs};
use std::io::Write;

#[derive(Deserialize)]
struct Input {
    url: String,
}

#[derive(Serialize)]
struct Output {
    #[serde(skip_serializing_if = "Option::is_none")]
    result: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

// Global variable to store the result
static mut RESULT: Option<Vec<u8>> = None;

#[no_mangle]
pub extern "C" fn allocate(size: usize) -> *mut u8 {
    let mut buf = Vec::with_capacity(size);
    let ptr = buf.as_mut_ptr();
    std::mem::forget(buf);
    ptr
}

#[no_mangle]
pub extern "C" fn deallocate(ptr: *mut u8, size: usize) {
    unsafe {
        let _ = Vec::from_raw_parts(ptr, 0, size);
    }
}

#[no_mangle]
pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) {
    unsafe {
        // Read input from memory
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

        // Parse input JSON
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

        // Attempt various network operations - all should be blocked by sandbox
        let output = {
            // 1. Try to connect to a TCP socket
            let tcp_result = match TcpStream::connect("8.8.8.8:53") {
                Ok(mut stream) => {
                    let _ = stream.write_all(b"test");
                    Some("Successfully connected to TCP socket".to_string())
                },
                Err(e1) => {
                    // 2. Try to resolve DNS
                    match "google.com:80".to_socket_addrs() {
                        Ok(mut addrs) => {
                            if let Some(addr) = addrs.next() {
                                Some(format!("Successfully resolved DNS: {}", addr))
                            } else {
                                None
                            }
                        },
                        Err(e2) => {
                            // 3. Try HTTP-like URL parsing (attempts to infer hostname)
                            if input.url.starts_with("http://") || input.url.starts_with("https://") {
                                match TcpStream::connect("1.1.1.1:443") {
                                    Ok(_) => Some("Successfully connected to fallback address".to_string()),
                                    Err(e3) => None,
                                }
                            } else {
                                None
                            }
                        }
                    }
                }
            };

            match tcp_result {
                Some(success) => Output {
                    result: Some(success),
                    error: None,
                },
                None => Output {
                    result: None,
                    error: Some(format!(
                        "All network access attempts blocked by sandbox for URL: {}",
                        input.url
                    )),
                },
            }
        };

        // Store result globally
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
