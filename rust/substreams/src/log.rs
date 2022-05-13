use crate::externs;

/// Logs a message at INFO level on current substream's logger using interpolation of
/// runtime expressions.
///
/// The behavior is exactly like [std::format::format!] built-in Rust formatting primitive.
///
/// # Panics
///
/// `format!` panics if a formatting trait implementation returns an error.
/// This indicates an incorrect implementation
/// since `fmt::Write for String` never returns an error itself.
///
/// # Examples
///
/// ```
/// info!("test");
/// info!("hello {}", "world!");
/// info!("x = {}, y = {y}", 10, y = 30);
/// ```
#[doc(hidden)]
#[macro_export]
macro_rules! log_info {
    // We have a special case when matching an expression directly to forward directly to `println`. This is to avoid
    // any allocation and pass directly the literal to `println` which is able to deal with. However, I'm wondering if
    // this will cause WTF moment for some cases.
    ($msg:expr) => {
        $crate::log::println($msg);
    };

    ($($arg:tt)*) => {{
        let message = std::fmt::format(format_args!($($arg)*));

        $crate::log::println(message);
    }}
}

/// Logs a message at DEBUG level on current substream's logger using interpolation of
/// runtime expressions.
///
/// The behavior is exactly like [std::format::format!] built-in Rust formatting primitive.
///
/// # Panics
///
/// `format!` panics if a formatting trait implementation returns an error.
/// This indicates an incorrect implementation
/// since `fmt::Write for String` never returns an error itself.
///
/// # Examples
///
/// ```
/// debug!("test");
/// debug!("hello {}", "world!");
/// debug!("x = {}, y = {y}", 10, y = 30);
/// ```
#[doc(hidden)]
#[macro_export]
macro_rules! log_debug {
    // We have a special case when matching an expression directly to forward directly to `println`. This is to avoid
    // any allocation and pass directly the literal to `println` which is able to deal with. However, I'm wondering if
    // this will cause WTF moment for some cases.
    ($msg:expr) => {
        $crate::log::println($msg);
    };

    ($($arg:tt)*) => {{
        let message = std::fmt::format(format_args!($($arg)*));

        $crate::log::println(message);
    }}
}

pub use log_debug as debug;
pub use log_info as info;

pub fn println<T: AsRef<str>>(msg: T) {
    let reference = msg.as_ref();

    unsafe {
        externs::println(reference.as_ptr(), reference.len());
    }
}
