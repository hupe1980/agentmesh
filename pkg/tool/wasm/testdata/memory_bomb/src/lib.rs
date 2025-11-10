use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
struct Input {
    size_mb: usize,
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

        // Attempt to allocate massive amounts of memory
        let size_bytes = input.size_mb * 1024 * 1024;
        let result = match std::panic::catch_unwind(|| {
            let mut memory_hog: Vec<u8> = Vec::with_capacity(size_bytes);
            memory_hog.resize(size_bytes, 0xFF);
            memory_hog.len()
        }) {
            Ok(allocated) => Output {
                result: Some(format!("Allocated {} bytes", allocated)),
                error: None,
            },
            Err(_) => Output {
                result: None,
                error: Some("Memory allocation failed (sandbox limit enforced)".to_string()),
            },
        };

        RESULT = Some(serde_json::to_vec(&result).unwrap());
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
