use crate::externs;

pub fn debug(msg: String) {
    unsafe {
        externs::debug(msg.as_ptr(), msg.len());
    }
}
pub fn info(msg: String) {
    unsafe {
        externs::info(msg.as_ptr(), msg.len());
    }
}
pub fn println(msg: String) {
    debug(msg);
}
