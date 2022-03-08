use crate::externs;

pub fn println(msg: String) {
    unsafe {
        externs::println(msg.as_ptr(), msg.len());
    }
}
