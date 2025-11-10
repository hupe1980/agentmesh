use serde::{Deserialize, Serialize};
use std::cell::RefCell;
use std::ptr;

thread_local! {
    // Thread-local storage is safe for single-threaded WASM and avoids `static mut` aliasing issues.
    static RESULT: RefCell<Option<Vec<u8>>> = RefCell::new(None);
}

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

#[no_mangle]
pub extern "C" fn allocate(size: usize) -> *mut u8 {
    // Return a pointer to uninitialized memory of capacity `size`.
    // Host must write exactly `size` bytes into this buffer.
    let mut buf = Vec::with_capacity(size);
    let ptr = buf.as_mut_ptr();
    std::mem::forget(buf);
    ptr
}

#[no_mangle]
pub extern "C" fn deallocate(ptr: *mut u8, size: usize) {
    // Reconstruct the Vec with length==capacity==size so it will be freed correctly.
    // If ptr is null or size == 0, nothing to do.
    if ptr.is_null() || size == 0 {
        return;
    }
    unsafe {
        // SAFETY: this assumes the pointer was allocated by `allocate` with the same `size`. 
        // The host must ensure that contract.
        let _ = Vec::from_raw_parts(ptr, size, size);
    }
}

#[no_mangle]
pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) {
    // Read input from WASM linear memory (unsafe only for the slice creation).
    let input_str = unsafe {
        if input_ptr.is_null() || input_len == 0 {
            let error_output = Output {
                result: None,
                error: Some("empty input".to_string()),
            };
            RESULT.with(|r| *r.borrow_mut() = Some(serde_json::to_vec(&error_output).unwrap()));
            return;
        }

        let input_slice = std::slice::from_raw_parts(input_ptr, input_len);
        match std::str::from_utf8(input_slice) {
            Ok(s) => s.to_owned(),
            Err(e) => {
                let error_output = Output {
                    result: None,
                    error: Some(format!("invalid UTF-8: {}", e)),
                };
                RESULT.with(|r| *r.borrow_mut() = Some(serde_json::to_vec(&error_output).unwrap()));
                return;
            }
        }
    };

    // Parse input JSON
    let input: Input = match serde_json::from_str(&input_str) {
        Ok(i) => i,
        Err(e) => {
            let error_output = Output {
                result: None,
                error: Some(format!("invalid JSON: {}", e)),
            };
            RESULT.with(|r| *r.borrow_mut() = Some(serde_json::to_vec(&error_output).unwrap()));
            return;
        }
    };

    // Evaluate the expression
    let output = match evaluate(&input.expression) {
        Ok(result) => Output {
            result: Some(result),
            error: None,
        },
        Err(e) => Output {
            result: None,
            error: Some(e),
        },
    };

    // Store result in thread-local storage
    RESULT.with(|r| *r.borrow_mut() = Some(serde_json::to_vec(&output).unwrap()));
}

#[no_mangle]
pub extern "C" fn get_result_ptr() -> *const u8 {
    // Return pointer to result bytes, or null if none.
    RESULT.with(|r| match &*r.borrow() {
        Some(vec) => vec.as_ptr(),
        None => ptr::null(),
    })
}

#[no_mangle]
pub extern "C" fn get_result_len() -> usize {
    // Return length of result bytes, or 0 if none.
    RESULT.with(|r| match &*r.borrow() {
        Some(vec) => vec.len(),
        None => 0,
    })
}

fn evaluate(expr: &str) -> Result<f64, String> {
    let expr = expr.trim();
    if expr.is_empty() {
        return Err("empty expression".to_string());
    }

    // Simple recursive descent parser for arithmetic expressions
    parse_addition(expr)
}

fn parse_addition(expr: &str) -> Result<f64, String> {
    let mut result = None;
    let mut op = '+';
    let mut current = String::new();
    let mut depth = 0;

    for ch in expr.chars() {
        if ch == '(' {
            depth += 1;
            current.push(ch);
        } else if ch == ')' {
            depth -= 1;
            if depth < 0 {
                return Err("unmatched parentheses".to_string());
            }
            current.push(ch);
        } else if depth == 0 && (ch == '+' || ch == '-') {
            let val = parse_multiplication(&current)?;
            current.clear();

            result = Some(match result {
                None => val,
                Some(prev) => match op {
                    '+' => prev + val,
                    '-' => prev - val,
                    _ => return Err("invalid operator".to_string()),
                },
            });
            op = ch;
        } else {
            current.push(ch);
        }
    }

    if depth != 0 {
        return Err("unmatched parentheses".to_string());
    }

    let val = parse_multiplication(&current)?;
    Ok(match result {
        None => val,
        Some(prev) => match op {
            '+' => prev + val,
            '-' => prev - val,
            _ => return Err("invalid operator".to_string()),
        },
    })
}

fn parse_multiplication(expr: &str) -> Result<f64, String> {
    let mut result = None;
    let mut op = '*';
    let mut current = String::new();
    let mut depth = 0;

    for ch in expr.chars() {
        if ch == '(' {
            depth += 1;
            current.push(ch);
        } else if ch == ')' {
            depth -= 1;
            current.push(ch);
        } else if depth == 0 && (ch == '*' || ch == '/') {
            let val = parse_primary(&current)?;
            current.clear();

            result = Some(match result {
                None => val,
                Some(prev) => match op {
                    '*' => prev * val,
                    '/' => {
                        if val == 0.0 {
                            return Err("division by zero".to_string());
                        }
                        prev / val
                    }
                    _ => return Err("invalid operator".to_string()),
                },
            });
            op = ch;
        } else {
            current.push(ch);
        }
    }

    let val = parse_primary(&current)?;
    Ok(match result {
        None => val,
        Some(prev) => match op {
            '*' => prev * val,
            '/' => {
                if val == 0.0 {
                    return Err("division by zero".to_string());
                }
                prev / val
            }
            _ => return Err("invalid operator".to_string()),
        },
    })
}

fn parse_primary(expr: &str) -> Result<f64, String> {
    let expr = expr.trim();

    // Handle parentheses
    if expr.starts_with('(') && expr.ends_with(')') {
        return parse_addition(&expr[1..expr.len() - 1]);
    }

    // Handle negative numbers
    if expr.starts_with('-') {
        let val = parse_primary(&expr[1..])?;
        return Ok(-val);
    }

    // Parse number
    expr.parse::<f64>()
        .map_err(|_| format!("invalid number: {}", expr))
}
