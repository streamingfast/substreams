use std::fmt;

/// Hex is a simple wrapper type that you can use to wrap your type so that it
/// prints in lower hexadecimal format when use as a formatting argument.
///
/// # Examples
///
/// ```
/// use substreams::Hex;
///
/// println!("Got an hex value {}", Hex(&[0xabu8, 0xcdu8, 0xefu8]));
/// ```
///
/// It can also be used directly to encode your type as a lower hexadecimal
/// `String`:
///
/// # Examples
///
/// ```
/// use substreams::Hex;
///
/// let encode = Hex::encode(&[0xabu8, 0xcdu8, 0xefu8]);
/// ```
pub struct Hex<T>(pub T);

impl<T: AsRef<[u8]>> Hex<T> {
    pub fn encode(input: T) -> String {
        encode_lower_hex(input.as_ref())
    }

    pub fn to_string(&self) -> String {
        encode_lower_hex(self.0.as_ref())
    }
}

impl<T: AsRef<[u8]>> fmt::Debug for Hex<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write_lower_hex(self.0.as_ref(), f)
    }
}

impl<T: AsRef<[u8]>> fmt::Display for Hex<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write_lower_hex(self.0.as_ref(), f)
    }
}

impl<T: AsRef<[u8]>> fmt::LowerHex for Hex<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write_lower_hex(self.0.as_ref(), f)
    }
}

const LOWER_HEX_TABLE: [&str; 16] = [
    "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f",
];

fn write_lower_hex(input: &[u8], mut w: impl std::fmt::Write) -> fmt::Result {
    for byte in input {
        let low_nibble = byte & 0x0F;
        let high_nibble = (byte & 0xF0) >> 4;

        w.write_str(LOWER_HEX_TABLE[high_nibble as usize])?;
        w.write_str(LOWER_HEX_TABLE[low_nibble as usize])?;
    }

    return Ok(());
}

fn encode_lower_hex<T: AsRef<[u8]>>(input: T) -> String {
    let bytes = input.as_ref();

    if bytes.len() == 0 {
        return "".to_string();
    }

    let mut buffer = String::with_capacity(bytes.len());

    write_lower_hex(bytes, &mut buffer).expect("non-faillible pre-allocated buffer");
    buffer
}

#[cfg(test)]
mod tests {
    use crate::hex::encode_lower_hex;

    #[test]
    fn it_encode_lower_hex_correctly() {
        assert_eq!(encode_lower_hex(&[] as &[u8; 0]), "");
        assert_eq!(encode_lower_hex(&[0x01u8]), "01");
        assert_eq!(encode_lower_hex(&[0xa1u8, 0xc3u8]), "a1c3");
    }
}
