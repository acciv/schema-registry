//! schemar-core — проверка совместимости версий API-схем.
//! Без внешних зависимостей. Сравнивает упрощённый JSON Schema.
//!
//! Правила: обратная совместимость означает, что новая схема принимает
//! все значения, которые принимала старая. С[idx] — путь в схеме.

use std::collections::BTreeMap;
use std::ffi::{c_char, CStr, CString};

pub const VERSION: &str = "0.1.0";

#[derive(Debug, Clone, PartialEq)]
enum Json {
    Str(String),
    Num(f64),
    Bool(bool),
    Obj(BTreeMap<String, Json>),
    Arr(Vec<Json>),
    Null,
}

fn skip_ws(bytes: &[u8], at: &mut usize) {
    while *at < bytes.len() && bytes[*at].is_ascii_whitespace() {
        *at += 1;
    }
}

fn parse_string(bytes: &[u8], at: &mut usize) -> Option<String> {
    if bytes.get(*at) != Some(&b'"') {
        return None;
    }
    *at += 1;
    let mut out = String::new();
    while *at < bytes.len() {
        let ch = bytes[*at];
        *at += 1;
        match ch {
            b'"' => return Some(out),
            b'\\' => {
                let next = *bytes.get(*at)?;
                *at += 1;
                match next {
                    b'n' => out.push('\n'),
                    b'r' => out.push('\r'),
                    b't' => out.push('\t'),
                    b'"' => out.push('"'),
                    b'\\' => out.push('\\'),
                    b'/' => out.push('/'),
                    b'b' => out.push('\u{8}'),
                    b'f' => out.push('\u{c}'),
                    b'u' => {
                        if *at + 4 > bytes.len() {
                            return None;
                        }
                        let hex = std::str::from_utf8(&bytes[*at..*at + 4]).ok()?;
                        let code = u32::from_str_radix(hex, 16).ok()?;
                        *at += 4;
                        out.push(char::from_u32(code).unwrap_or('\u{fffd}'));
                    }
                    _ => out.push(next as char),
                }
            }
            _ => out.push(ch as char),
        }
    }
    None
}

fn parse_value(bytes: &[u8], at: &mut usize) -> Option<Json> {
    skip_ws(bytes, at);
    match bytes.get(*at)? {
        b'"' => parse_string(bytes, at).map(Json::Str),
        b'{' => {
            *at += 1;
            let mut map = BTreeMap::new();
            loop {
                skip_ws(bytes, at);
                if bytes.get(*at) == Some(&b'}') {
                    *at += 1;
                    return Some(Json::Obj(map));
                }
                let key = parse_string(bytes, at)?;
                skip_ws(bytes, at);
                if bytes.get(*at) != Some(&b':') {
                    return None;
                }
                *at += 1;
                map.insert(key, parse_value(bytes, at)?);
                skip_ws(bytes, at);
                match bytes.get(*at) {
                    Some(b',') => *at += 1,
                    Some(b'}') => {
                        *at += 1;
                        return Some(Json::Obj(map));
                    }
                    _ => return None,
                }
            }
        }
        b'[' => {
            *at += 1;
            let mut items = Vec::new();
            loop {
                skip_ws(bytes, at);
                if bytes.get(*at) == Some(&b']') {
                    *at += 1;
                    return Some(Json::Arr(items));
                }
                items.push(parse_value(bytes, at)?);
                skip_ws(bytes, at);
                match bytes.get(*at) {
                    Some(b',') => *at += 1,
                    Some(b']') => {
                        *at += 1;
                        return Some(Json::Arr(items));
                    }
                    _ => return None,
                }
            }
        }
        b't' if bytes[*at..].starts_with(b"true") => {
            *at += 4;
            Some(Json::Bool(true))
        }
        b'f' if bytes[*at..].starts_with(b"false") => {
            *at += 5;
            Some(Json::Bool(false))
        }
        b'n' if bytes[*at..].starts_with(b"null") => {
            *at += 4;
            Some(Json::Null)
        }
        b'-' | b'0'..=b'9' => {
            let start = *at;
            while *at < bytes.len()
                && (bytes[*at].is_ascii_digit()
                    || matches!(bytes[*at], b'-' | b'+' | b'.' | b'e' | b'E'))
            {
                *at += 1;
            }
            std::str::from_utf8(&bytes[start..*at])
                .ok()?
                .parse::<f64>()
                .ok()
                .map(Json::Num)
        }
        _ => None,
    }
}

fn parse_json(input: &str) -> Option<Json> {
    let bytes = input.as_bytes();
    let mut at = 0usize;
    let value = parse_value(bytes, &mut at)?;
    skip_ws(bytes, &mut at);
    if at == bytes.len() {
        Some(value)
    } else {
        None
    }
}

pub fn json_escape(input: &str) -> String {
    let mut out = String::with_capacity(input.len() + 2);
    for ch in input.chars() {
        match ch {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if c.is_control() => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out
}

fn jstr(input: &str) -> String {
    format!("\"{}\"", json_escape(input))
}

fn to_json(value: &Json) -> String {
    match value {
        Json::Str(s) => jstr(s),
        Json::Num(n) if n.fract() == 0.0 && n.is_finite() => format!("{}", *n as i64),
        Json::Num(n) => format!("{}", n),
        Json::Bool(b) => b.to_string(),
        Json::Null => "null".into(),
        Json::Arr(items) => format!("[{}]", items.iter().map(to_json).collect::<Vec<_>>().join(",")),
        Json::Obj(map) => format!(
            "{{{}}}",
            map.iter()
                .map(|(k, v)| format!("{}:{}", jstr(k), to_json(v)))
                .collect::<Vec<_>>()
                .join(",")
        ),
    }
}

fn as_obj(value: &Json) -> Option<&BTreeMap<String, Json>> {
    match value {
        Json::Obj(map) => Some(map),
        _ => None,
    }
}

fn field<'a>(map: &'a BTreeMap<String, Json>, key: &str) -> Option<&'a Json> {
    map.get(key)
}

fn field_str(map: &BTreeMap<String, Json>, key: &str) -> String {
    match field(map, key) {
        Some(Json::Str(s)) => s.clone(),
        Some(Json::Num(n)) => format!("{}", n),
        _ => String::new(),
    }
}

fn field_array(map: &BTreeMap<String, Json>, key: &str) -> Vec<Json> {
    match field(map, key) {
        Some(Json::Arr(items)) => items.clone(),
        _ => Vec::new(),
    }
}

fn json_type(value: &Json) -> &'static str {
    match value {
        Json::Str(_) => "string",
        Json::Num(_) => "number",
        Json::Bool(_) => "boolean",
        Json::Obj(_) => "object",
        Json::Arr(_) => "array",
        Json::Null => "null",
    }
}

fn type_of(map: &BTreeMap<String, Json>) -> String {
    let declared = field_str(map, "type");
    if !declared.is_empty() {
        return declared;
    }
    if field(map, "properties").is_some() {
        return "object".into();
    }
    if field(map, "items").is_some() {
        return "array".into();
    }
    if let Some(first) = field_array(map, "enum").first() {
        return json_type(first).to_string();
    }
    "any".into()
}

fn numeric_widening(old: &str, new: &str) -> bool {
    old == "integer" && new == "number"
}

fn fnv64(input: &str) -> u64 {
    let mut hash = 0xcbf2_9ce4_8422_2325u64;
    for byte in input.bytes() {
        hash ^= byte as u64;
        hash = hash.wrapping_mul(0x0100_0000_01b3);
    }
    hash
}

type Change = BTreeMap<String, Json>;

fn change(path: &str, kind: &str, breaking: bool, from: &str, to: &str) -> Change {
    let mut item = BTreeMap::new();
    item.insert("path".into(), Json::Str(path.into()));
    item.insert("kind".into(), Json::Str(kind.into()));
    item.insert("breaking".into(), Json::Bool(breaking));
    if !from.is_empty() {
        item.insert("from".into(), Json::Str(from.into()));
    }
    if !to.is_empty() {
        item.insert("to".into(), Json::Str(to.into()));
    }
    item
}

fn compare_enum(path: &str, old: &BTreeMap<String, Json>, new: &BTreeMap<String, Json>, changes: &mut Vec<Change>) {
    let old_enum = field_array(old, "enum");
    let new_enum = field_array(new, "enum");
    if old_enum.is_empty() {
        if !new_enum.is_empty() {
            changes.push(change(path, "enum_narrowed", true, "any", "enum"));
        }
        return;
    }
    if new_enum.is_empty() {
        changes.push(change(path, "enum_removed", false, "enum", "any"));
        return;
    }
    for value in &old_enum {
        if !new_enum.contains(value) {
            changes.push(change(path, "enum_value_removed", true, &to_json(value), ""));
        }
    }
    for value in &new_enum {
        if !old_enum.contains(value) {
            changes.push(change(path, "enum_value_added", false, "", &to_json(value)));
        }
    }
}

fn compare_node(path: &str, old: &Json, new: &Json, changes: &mut Vec<Change>) {
    let (Some(old_map), Some(new_map)) = (as_obj(old), as_obj(new)) else {
        if old != new {
            changes.push(change(path, "schema_changed", true, &to_json(old), &to_json(new)));
        }
        return;
    };

    let old_type = type_of(old_map);
    let new_type = type_of(new_map);
    if old_type != new_type {
        if numeric_widening(&old_type, &new_type) {
            changes.push(change(path, "type_widened", false, &old_type, &new_type));
        } else {
            changes.push(change(path, "type_changed", true, &old_type, &new_type));
        }
        return;
    }

    match old_type.as_str() {
        "object" => {
            let old_props = match field(old_map, "properties").and_then(as_obj) {
                Some(map) => map.clone(),
                None => BTreeMap::new(),
            };
            let new_props = match field(new_map, "properties").and_then(as_obj) {
                Some(map) => map.clone(),
                None => BTreeMap::new(),
            };
            let old_required = field_array(old_map, "required");
            let new_required = field_array(new_map, "required");

            for key in old_props.keys() {
                if !new_props.contains_key(key) {
                    let breaking = old_required.iter().any(|v| v == &Json::Str(key.clone()));
                    changes.push(change(&format!("{}.{}", path, key), "property_removed", breaking, "present", "missing"));
                }
            }
            for key in new_props.keys() {
                if !old_props.contains_key(key) {
                    let breaking = new_required.iter().any(|v| v == &Json::Str(key.clone()));
                    changes.push(change(&format!("{}.{}", path, key), "property_added", breaking, "missing", "present"));
                }
            }
            for (key, old_value) in &old_props {
                if let Some(new_value) = new_props.get(key) {
                    compare_node(&format!("{}.{}", path, key), old_value, new_value, changes);
                }
            }
            for value in &new_required {
                let Json::Str(key) = value else { continue };
                if !old_required.contains(value) && old_props.contains_key(key) {
                    changes.push(change(&format!("{}.{}", path, key), "became_required", true, "optional", "required"));
                }
            }
            for value in &old_required {
                if !new_required.contains(value) {
                    let key = match value {
                        Json::Str(s) => s.clone(),
                        other => to_json(other),
                    };
                    changes.push(change(&format!("{}.{}", path, key), "became_optional", false, "required", "optional"));
                }
            }
        }
        "array" => {
            let old_items = field(old_map, "items").cloned();
            let new_items = field(new_map, "items").cloned();
            match (old_items, new_items) {
                (Some(o), Some(n)) => compare_node(&format!("{}[]", path), &o, &n, changes),
                (Some(_), None) => changes.push(change(&format!("{}[]", path), "items_removed", true, "typed", "any")),
                (None, Some(_)) => changes.push(change(&format!("{}[]", path), "items_added", true, "any", "typed")),
                (None, None) => {}
            }
        }
        _ => {}
    }

    compare_enum(path, old_map, new_map, changes);
}

/// Сравнивает две схемы. Возвращает JSON-отчёт о совместимости.
pub fn check(old_input: &str, new_input: &str) -> Result<String, String> {
    let Some(old_schema) = parse_json(old_input) else {
        return Err("invalid old schema JSON".into());
    };
    let Some(new_schema) = parse_json(new_input) else {
        return Err("invalid new schema JSON".into());
    };
    if as_obj(&old_schema).is_none() || as_obj(&new_schema).is_none() {
        return Err("schema must be a JSON object".into());
    }

    let mut changes = Vec::new();
    compare_node("$", &old_schema, &new_schema, &mut changes);
    let breaking = changes.iter().filter(|c| {
        matches!(c.get("breaking"), Some(Json::Bool(true)))
    }).count();

    let mut out = BTreeMap::new();
    out.insert("compatible".into(), Json::Bool(breaking == 0));
    out.insert("breaking".into(), Json::Num(breaking as f64));
    out.insert(
        "changes".into(),
        Json::Arr(changes.into_iter().map(Json::Obj).collect()),
    );
    Ok(to_json(&Json::Obj(out)))
}

/// Стабильный отпечаток схемы: порядок ключей не важен.
pub fn fingerprint(input: &str) -> Result<String, String> {
    let Some(value) = parse_json(input) else {
        return Err("invalid schema JSON".into());
    };
    Ok(format!("{:016x}", fnv64(&to_json(&value))))
}

fn read_input(ptr: *const c_char) -> Option<String> {
    if ptr.is_null() {
        return None;
    }
    unsafe { CStr::from_ptr(ptr) }.to_str().ok().map(str::to_string)
}

fn error(message: &str) -> String {
    format!("{{\"error\":\"{}\"}}", json_escape(message))
}

fn to_c(value: String) -> *mut c_char {
    CString::new(value)
        .unwrap_or_else(|_| CString::new("{\"error\":\"invalid string\"}").unwrap())
        .into_raw()
}

#[no_mangle]
pub extern "C" fn schemar_version() -> *const c_char {
    concat!(env!("CARGO_PKG_VERSION"), "\0").as_ptr() as *const c_char
}

#[no_mangle]
pub extern "C" fn schemar_check(old_ptr: *const c_char, new_ptr: *const c_char) -> *mut c_char {
    let result = match (read_input(old_ptr), read_input(new_ptr)) {
        (Some(old), Some(new)) => check(&old, &new).unwrap_or_else(|e| error(&e)),
        _ => error("null input"),
    };
    to_c(result)
}

#[no_mangle]
pub extern "C" fn schemar_fingerprint(input: *const c_char) -> *mut c_char {
    let result = match read_input(input) {
        Some(value) => match fingerprint(&value) {
            Ok(fp) => format!("{{\"fingerprint\":\"{}\"}}", fp),
            Err(err) => error(&err),
        },
        None => error("null input"),
    };
    to_c(result)
}

#[no_mangle]
pub extern "C" fn schemar_free(ptr: *mut c_char) {
    if !ptr.is_null() {
        unsafe {
            drop(CString::from_raw(ptr));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const V1: &str = r#"{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}"#;

    #[test]
    fn adding_optional_property_is_compatible() {
        let v2 = r#"{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"}},"required":["id"]}"#;
        let report: Json = parse_json(&check(V1, v2).unwrap()).unwrap();
        let map = as_obj(&report).unwrap();
        assert_eq!(map.get("compatible"), Some(&Json::Bool(true)));
    }

    #[test]
    fn adding_required_property_is_breaking() {
        let v2 = r#"{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"}},"required":["id","name"]}"#;
        let report = check(V1, v2).unwrap();
        assert!(report.contains("\"compatible\":false"));
        assert!(report.contains("property_added"));
    }

    #[test]
    fn type_change_is_breaking() {
        let v2 = r#"{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}"#;
        let report = check(V1, v2).unwrap();
        assert!(report.contains("type_changed"));
    }

    #[test]
    fn integer_to_number_is_widening() {
        let v1 = r#"{"type":"object","properties":{"n":{"type":"integer"}}}"#;
        let v2 = r#"{"type":"object","properties":{"n":{"type":"number"}}}"#;
        let report = check(v1, v2).unwrap();
        assert!(report.contains("type_widened"));
        assert!(report.contains("\"compatible\":true"));
    }

    #[test]
    fn fingerprint_ignores_key_order() {
        let a = r#"{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"number"}}}"#;
        let b = r#"{"properties":{"a":{"type":"number"},"b":{"type":"string"}},"type":"object"}"#;
        assert_eq!(fingerprint(a).unwrap(), fingerprint(b).unwrap());
    }

    #[test]
    fn invalid_json_is_error() {
        assert!(check("nope", V1).is_err());
        assert!(fingerprint("nope").is_err());
    }
}
