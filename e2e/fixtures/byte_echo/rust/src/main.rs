fn echo(data: &[u8]) -> Vec<u8> {
    let mut out = vec![0u8; data.len()];
    for (i, &b) in data.iter().enumerate() {
        if b < 0x20      { out[i] = b; }
        else if b < 0x40 { out[i] = b; }
        else if b < 0x60 { out[i] = b; }
        else if b < 0x80 { out[i] = b; }
        else             { out[i] = b; }
    }
    out
}

fn main() {
    crossfuzz::fuzz(|input| Ok(echo(input)), Default::default());
}
