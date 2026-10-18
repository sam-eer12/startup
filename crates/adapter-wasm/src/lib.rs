//! ABI v1: all pointers are wasm32 offsets. A fresh instance owns each invocation.
//! The trusted host allocates/copies input and output buffers and frees both.
use adapter_core::MAX_ENVELOPE;

#[unsafe(no_mangle)]
pub extern "C" fn abi_version() -> u32 {
    1
}

#[unsafe(no_mangle)]
pub extern "C" fn alloc(len: u32) -> u32 {
    if len == 0 || len as usize > MAX_ENVELOPE {
        return 0;
    }
    let bytes = vec![0u8; len as usize].into_boxed_slice();
    Box::into_raw(bytes) as *mut u8 as usize as u32
}

/// # Safety
/// Host must pass an outstanding alloc pointer with its exact allocation length.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dealloc(ptr: u32, len: u32) {
    if ptr != 0 && len != 0 {
        unsafe {
            drop(Box::from_raw(std::ptr::slice_from_raw_parts_mut(
                ptr as usize as *mut u8,
                len as usize,
            )));
        }
    }
}

/// Writes into a caller-owned bounded buffer. Negative return means insufficient
/// capacity/invalid length; otherwise return the number of initialized bytes.
/// # Safety
/// Input/output must be distinct live allocations of the supplied lengths.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn transform(ptr: u32, len: u32, out: u32, capacity: u32) -> i32 {
    if ptr == 0 || out == 0 || len as usize > MAX_ENVELOPE || capacity as usize > MAX_ENVELOPE {
        return -1;
    }
    let input = unsafe { std::slice::from_raw_parts(ptr as usize as *const u8, len as usize) };
    let output = adapter_core::execute(input);
    if output.len() > capacity as usize {
        return -1;
    }
    unsafe {
        std::ptr::copy_nonoverlapping(output.as_ptr(), out as usize as *mut u8, output.len());
    }
    output.len() as i32
}
