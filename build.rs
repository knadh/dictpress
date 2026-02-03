fn main() {
    let version = std::env::var("VERSION").unwrap_or_else(|_| "v0.0.0".to_string());
    println!("cargo:rustc-env=VERSION={}", version);
}
