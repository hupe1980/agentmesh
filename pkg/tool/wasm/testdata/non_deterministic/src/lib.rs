use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
struct Input {
    expression: String,
}

#[derive(Serialize)]
struct Output {
    #[serde(skip_serializing_if = "Option::is_none")]
    result: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

static mut RESULT: Option<Vec<u8>> = None;
static mut STATE: u64 = 0;

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

        // Intentionally use global state to break determinism
        STATE += 1;
        
        // Return different results based on state
        let result_value = if input.expression == "2 + 2" {
            4.0 + (STATE as f64)
        } else {
            STATE as f64
        };

        let output = Output {
            result: Some(result_value),
            error: None,
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
