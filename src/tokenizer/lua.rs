use std::{path::Path, sync::Mutex};

use mlua::{Function, Lua};

use super::{Tokenizer, TokenizerError};

/// Lua helper functions injected into every tokenizer script.
const LUA_GLOBAL: &[u8] = include_bytes!("global.lua");

/// Lua-based tokenizer loaded from a .lua file.
pub struct LuaTokenizer {
    lua: Mutex<Lua>,
}

impl LuaTokenizer {
    pub fn from_file(path: &Path) -> Result<Self, TokenizerError> {
        let lua = Lua::new();
        let code = std::fs::read_to_string(path)?;

        lua.load(LUA_GLOBAL).exec()?;
        lua.load(&code).exec()?;

        Ok(Self {
            lua: Mutex::new(lua),
        })
    }

    fn call_tokenize(&self, text: &str, lang: &str) -> Result<Vec<String>, TokenizerError> {
        let lua = self.lua.lock().unwrap();
        let globals = lua.globals();
        let func: Function = globals.get("tokenize")?;
        let tokens: Vec<String> = func.call((text, lang))?;
        Ok(tokens)
    }

    fn call_query(&self, text: &str, lang: &str) -> Result<String, TokenizerError> {
        let lua = self.lua.lock().unwrap();
        let globals = lua.globals();
        let func: Function = globals.get("to_query")?;
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
