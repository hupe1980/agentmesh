use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
struct Input {
    a: f64,
    b: f64,
    operation: String,
}

#[derive(Serialize)]
struct Output {
    #[serde(skip_serializing_if = "Option::is_none")]
    result: Option<f64>,
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

        // Perform simple mathematical operations
        let result = match input.operation.as_str() {
            "add" => input.a + input.b,
            "subtract" => input.a - input.b,
            "multiply" => input.a * input.b,
            "divide" => {
                if input.b == 0.0 {
                    let error_output = Output {
                        result: None,
                        error: Some("division by zero".to_string()),
                    };
                    RESULT = Some(serde_json::to_vec(&error_output).unwrap());
                    return;
                }
                input.a / input.b
            }
            "power" => input.a.powf(input.b),
            "modulo" => input.a % input.b,
            _ => {
                let error_output = Output {
                    result: None,
                    error: Some(format!("unknown operation: {}", input.operation)),
                };
                RESULT = Some(serde_json::to_vec(&error_output).unwrap());
                return;
            }
        };

        let output = Output {
            result: Some(result),
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
