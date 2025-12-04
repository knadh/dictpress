use std::{path::Path, sync::Mutex};

use mlua::{Function, Lua, Table};

use super::{Tokenizer, TokenizerError};

/// Lua helper functions injected into every tokenizer script.
const LUA_HELPERS: &str = r#"
-- Helper library available as `dp` in all tokenizer scripts.
dp = {}

-- Split string by whitespace, returns iterator.
function dp.words(s)
    return s:gmatch("%S+")
end

-- Lowercase string.
function dp.lower(s)
    return s:lower()
end

-- Trim whitespace.
function dp.trim(s)
    return s:match("^%s*(.-)%s*$")
end

-- Split string by delimiter.
function dp.split(s, delim)
    local result = {}
    for match in (s .. delim):gmatch("(.-)" .. delim) do
        table.insert(result, match)
    end
    return result
end
"#;

/// Lua-based tokenizer loaded from a .lua file.
pub struct LuaTokenizer {
    lua: Mutex<Lua>,
}

impl LuaTokenizer {
    pub fn from_file(path: &Path) -> Result<Self, TokenizerError> {
        let lua = Lua::new();
        let code = std::fs::read_to_string(path)?;

        // Load helper library first.
        lua.load(LUA_HELPERS).exec()?;

        // Load user tokenizer script. Script should assign to global `M` table.
        lua.load(&code).exec()?;

        Ok(Self {
            lua: Mutex::new(lua),
        })
    }

    fn call_tokenize(&self, text: &str, lang: &str) -> Result<Vec<String>, TokenizerError> {
        let lua = self.lua.lock().unwrap();
        let globals = lua.globals();
        let module: Table = globals.get("M")?;
        let func: Function = module.get("tokenize")?;
        let tokens: Vec<String> = func.call((text, lang))?;
        Ok(tokens)
    }

    fn call_query(&self, text: &str, lang: &str) -> Result<String, TokenizerError> {
        let lua = self.lua.lock().unwrap();
        let globals = lua.globals();
        let module: Table = globals.get("M")?;
        let func: Function = module.get("query")?;
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
