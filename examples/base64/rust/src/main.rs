fn encode_base64(data: &[u8]) -> Vec<u8> {
    const ALPHABET: &[u8] =
        b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

    let out_len = 4 * ((data.len() + 2) / 3);
    let mut out = vec![0u8; out_len];

    let mut i = 0;
    let mut j = 0;

    while i < data.len() {
        let a = data[i] as u32;
        i += 1;
        let b = if i < data.len() { let v = data[i] as u32; i += 1; v } else { 0 };
        let c = if i < data.len() { let v = data[i] as u32; i += 1; v } else { 0 };

        let triple = (a << 16) | (b << 8) | c;
        out[j]     = ALPHABET[((triple >> 18) & 0x3F) as usize];
        out[j + 1] = ALPHABET[((triple >> 12) & 0x3F) as usize];
        out[j + 2] = ALPHABET[((triple >>  6) & 0x3F) as usize];
        out[j + 3] = ALPHABET[( triple        & 0x3F) as usize];
        j += 4;
    }

    // Padding
    let rem = data.len() % 3;
    if rem == 1 {
        out[out_len - 2] = b'=';
        out[out_len - 1] = b'=';
    } else if rem == 2 {
        out[out_len - 1] = b'=';
    }

    out
}

fn main() {
    crossfuzz::fuzz(|input| Ok(encode_base64(input)), Default::default());
}
