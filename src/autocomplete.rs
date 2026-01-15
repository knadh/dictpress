use std::collections::HashMap;

use trie_rs::{Trie, TrieBuilder};

/// Normalize a word: lowercase, strip numbers, replace dashes with spaces.
pub fn normalize_word(s: &str) -> String {
    s.to_lowercase()
        .replace('-', " ")
        .chars()
        .filter(|c| !c.is_numeric())
        .collect()
}

/// In-memory trie-based autocomplete for fast prefix matching.
pub struct Autocomplete {
    tries: HashMap<String, Trie<u8>>,
}

impl Autocomplete {
    pub fn new() -> Self {
        Self {
            tries: HashMap::new(),
        }
    }

    /// Build trie for a language from words. Words are normalized and sorted before insertion.
    pub fn build(&mut self, lang: &str, words: Vec<String>) {
        let mut b = TrieBuilder::new();
        for w in words {
            let wn = normalize_word(&w);
            if !wn.is_empty() {
                b.push(wn);
            }
        }
        self.tries.insert(lang.to_string(), b.build());
    }

    /// Query autocomplete results for a prefix (normalizes the prefix internally).
    /// Returns up to `num` matching words.
    pub fn query(&self, lang: &str, prefix: &str, num: usize) -> Vec<String> {
        let word = normalize_word(prefix);
        if word.is_empty() {
            return Vec::new();
        }

        let trie = match self.tries.get(lang) {
            Some(t) => t,
            None => return Vec::new(),
        };

        let out: Vec<String> = trie.predictive_search(&word).take(num).collect();
        out
    }
}
