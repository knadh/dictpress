use std::{path::Path, sync::Mutex};

use mlua::{Function, Lua, RegistryKey};

use super::{Tokenizer, TokenizerError};

/// Lua helper functions injected into every tokenizer script.
const LUA_GLOBAL: &[u8] = include_bytes!("global.lua");

/// Lua-based tokenizer loaded from a .lua file.
pub struct LuaTokenizer {
    lua: Mutex<Lua>,
    tokenize_fn: RegistryKey,
    to_query_fn: RegistryKey,
}

impl LuaTokenizer {
    pub fn from_file(path: &Path) -> Result<Self, TokenizerError> {
        let lua = Lua::new();
        let code = std::fs::read_to_string(path)?;

        lua.load(LUA_GLOBAL).exec()?;
        lua.load(&code).exec()?;

        // Cache function references in the registry to avoid globals lookup on every call.
        let globals = lua.globals();
        let tokenize_fn: Function = globals.get("tokenize")?;
        let to_query_fn: Function = globals.get("to_query")?;
        let tokenize_key = lua.create_registry_value(tokenize_fn)?;
        let to_query_key = lua.create_registry_value(to_query_fn)?;

        Ok(Self {
            lua: Mutex::new(lua),
            tokenize_fn: tokenize_key,
            to_query_fn: to_query_key,
        })
    }

    fn call_tokenize(&self, text: &str, lang: &str) -> Result<Vec<String>, TokenizerError> {
        let lua = self.lua.lock().unwrap();
        let func: Function = lua.registry_value(&self.tokenize_fn)?;
        let tokens: Vec<String> = func.call((text, lang))?;
        Ok(tokens)
    }

    fn call_query(&self, text: &str, lang: &str) -> Result<String, TokenizerError> {
        let lua = self.lua.lock().unwrap();
        let func: Function = lua.registry_value(&self.to_query_fn)?;
        let query: String = func.call((text, lang))?;
        Ok(query)
    }
}

impl Tokenizer for LuaTokenizer {
    fn tokenize(&self, text: &str, lang: &str) -> Result<Vec<String>, TokenizerError> {
        self.call_tokenize(text, lang)
    }

    fn to_query(&self, text: &str, lang: &str) -> Result<String, TokenizerError> {
        self.call_query(text, lang)
    }
}
