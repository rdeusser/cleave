package rust

func (w *writer) writeRuntime() {
	w.sb.WriteString(`#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Error {
    UnexpectedEof {
        offset: usize,
        needed: usize,
        remaining: usize,
    },
    InvalidEnum {
        enum_name: &'static str,
        value: i128,
    },
    UnknownTag {
        struct_name: &'static str,
        field: &'static str,
        value: i128,
    },
    Validation {
        id: &'static str,
        message: &'static str,
        field: &'static str,
    },
    UnsupportedEncoding {
        encoding: &'static str,
    },
    InvalidEncoding {
        encoding: &'static str,
    },
    InvalidRegex {
        pattern: ::std::string::String,
        message: ::std::string::String,
    },
    InvalidLength {
        field: &'static str,
        value: i128,
    },
    LengthOverflow {
        field: &'static str,
        value: ::std::string::String,
        target: &'static str,
    },
    FixedSize {
        field: &'static str,
        expected: usize,
        actual: usize,
    },
    MatchType {
        field: &'static str,
        tag: i128,
    },
    NoProgress {
        field: &'static str,
        offset: usize,
    },
}

impl ::std::fmt::Display for Error {
    fn fmt(&self, formatter: &mut ::std::fmt::Formatter<'_>) -> ::std::fmt::Result {
        match self {
            Self::UnexpectedEof {
                offset,
                needed,
                remaining,
            } => write!(
                formatter,
                "unexpected end of input at offset {offset}: need {needed} bytes, have {remaining}",
            ),
            Self::InvalidEnum { enum_name, value } => {
                write!(formatter, "invalid {enum_name} value {value}")
            }
            Self::UnknownTag {
                struct_name,
                field,
                value,
            } => {
                write!(formatter, "unknown tag {value} for {struct_name}.{field}")
            }
            Self::Validation { id, message, field } => {
                write!(formatter, "validation {id} failed for {field}: {message}")
            }
            Self::UnsupportedEncoding { encoding } => {
                write!(formatter, "unsupported encoding {encoding}")
            }
            Self::InvalidEncoding { encoding } => {
                write!(formatter, "invalid {encoding} text")
            }
            Self::InvalidRegex { pattern, message } => {
                write!(
                    formatter,
                    "invalid regular expression {pattern:?}: {message}"
                )
            }
            Self::InvalidLength { field, value } => {
                write!(formatter, "invalid length {value} for {field}")
            }
            Self::LengthOverflow {
                field,
                value,
                target,
            } => {
                write!(
                    formatter,
                    "length {value} for {field} does not fit {target}"
                )
            }
            Self::FixedSize {
                field,
                expected,
                actual,
            } => write!(
                formatter,
                "{field} has length {actual}, expected {expected}",
            ),
            Self::MatchType { field, tag } => {
                write!(formatter, "payload for {field} does not match tag {tag}")
            }
            Self::NoProgress { field, offset } => {
                write!(
                    formatter,
                    "parsing {field} made no progress at offset {offset}"
                )
            }
        }
    }
}

impl ::std::error::Error for Error {}

#[allow(dead_code, reason = "some schemas do not read byte slices directly")]
fn take<'a>(
    buf: &'a [u8],
    offset: &mut usize,
    length: usize,
) -> ::std::result::Result<&'a [u8], Error> {
    let start = *offset;
    let end = start
        .checked_add(length)
        .ok_or_else(|| Error::LengthOverflow {
            field: "offset",
            value: length.to_string(),
            target: "usize",
        })?;
    let remaining = buf.len().saturating_sub(start);
    let bytes = buf.get(start..end).ok_or(Error::UnexpectedEof {
        offset: start,
        needed: length,
        remaining,
    })?;
    *offset = end;
    Ok(bytes)
}

#[allow(dead_code, reason = "some schemas do not contain numeric fields")]
fn read_array<const N: usize>(
    buf: &[u8],
    offset: &mut usize,
) -> ::std::result::Result<[u8; N], Error> {
    let bytes = take(buf, offset, N)?;
    <[u8; N]>::try_from(bytes).map_err(|_| Error::FixedSize {
        field: "primitive",
        expected: N,
        actual: bytes.len(),
    })
}

`)
	if w.requirements.hasText {
		w.writeEncodingRuntime()
		w.sb.WriteString("\n")
	}
	w.sb.WriteString(`#[allow(dead_code, reason = "empty schemas have no JSON strings to write")]
fn write_json_string(json: &mut ::std::string::String, value: &str) {
    json.push('"');
    for character in value.chars() {
        match character {
            '"' => json.push_str("\\\""),
            '\\' => json.push_str("\\\\"),
            '\u{0008}' => json.push_str("\\b"),
            '\u{000c}' => json.push_str("\\f"),
            '\n' => json.push_str("\\n"),
            '\r' => json.push_str("\\r"),
            '\t' => json.push_str("\\t"),
            control if control <= '\u{001f}' => {
                json.push_str("\\u");
                let code = u32::from(control);
                for shift in [12_u32, 8, 4, 0] {
                    if let ::std::option::Option::Some(digit) =
                        ::std::char::from_digit((code >> shift) & 0x0f, 16)
                    {
                        json.push(digit);
                    }
                }
            }
            other => json.push(other),
        }
    }
    json.push('"');
}
`)
	if w.requirements.hasStartsWith {
		w.sb.WriteString(`
fn text_starts_with(value: &str, prefix: &str) -> bool {
    value.starts_with(prefix)
}
`)
	}
	if w.requirements.hasEndsWith {
		w.sb.WriteString(`
fn text_ends_with(value: &str, suffix: &str) -> bool {
    value.ends_with(suffix)
}
`)
	}
	if w.requirements.hasContains {
		w.sb.WriteString(`
fn text_contains(value: &str, needle: &str) -> bool {
    value.contains(needle)
}
`)
	}
	if w.requirements.hasRegex {
		w.sb.WriteString(`
fn regex_matches(value: &str, pattern: &str) -> ::std::result::Result<bool, Error> {
    let expression = ::regex::Regex::new(pattern).map_err(|error| Error::InvalidRegex {
        pattern: pattern.to_owned(),
        message: error.to_string(),
    })?;
    Ok(expression.is_match(value))
}
`)
	}
}

func (w *writer) writeEncodingRuntime() {
	if !w.requirements.hasLegacyEncoding {
		w.sb.WriteString(`fn decode_text(
    bytes: &[u8],
    encoding: &'static str,
) -> ::std::result::Result<::std::string::String, Error> {
    if encoding.eq_ignore_ascii_case("utf-8") || encoding.eq_ignore_ascii_case("utf8") {
        return ::std::str::from_utf8(bytes)
            .map(::std::primitive::str::to_owned)
            .map_err(|_| Error::InvalidEncoding { encoding });
    }
    Err(Error::UnsupportedEncoding { encoding })
}

fn encode_text(
    value: &str,
    encoding: &'static str,
) -> ::std::result::Result<::std::vec::Vec<u8>, Error> {
    if encoding.eq_ignore_ascii_case("utf-8") || encoding.eq_ignore_ascii_case("utf8") {
        return Ok(value.as_bytes().to_vec());
    }
    Err(Error::UnsupportedEncoding { encoding })
}
`)
		return
	}

	w.sb.WriteString(`fn legacy_encoding(encoding: &str) -> ::std::option::Option<&'static ::encoding_rs::Encoding> {
    let label = if encoding.eq_ignore_ascii_case("cp949") {
        "windows-949"
    } else if encoding.eq_ignore_ascii_case("shift_jis") {
        "sjis"
    } else {
        encoding
    };
    ::encoding_rs::Encoding::for_label(label.as_bytes())
}

fn decode_text(
    bytes: &[u8],
    encoding: &'static str,
) -> ::std::result::Result<::std::string::String, Error> {
    if encoding.eq_ignore_ascii_case("utf-8") || encoding.eq_ignore_ascii_case("utf8") {
        return ::std::str::from_utf8(bytes)
            .map(::std::primitive::str::to_owned)
            .map_err(|_| Error::InvalidEncoding { encoding });
    }

    let codec = legacy_encoding(encoding).ok_or(Error::UnsupportedEncoding { encoding })?;
    let (decoded, had_errors) = codec.decode_without_bom_handling(bytes);
    if had_errors {
        return Err(Error::InvalidEncoding { encoding });
    }
    Ok(decoded.into_owned())
}

fn encode_text(
    value: &str,
    encoding: &'static str,
) -> ::std::result::Result<::std::vec::Vec<u8>, Error> {
    if encoding.eq_ignore_ascii_case("utf-8") || encoding.eq_ignore_ascii_case("utf8") {
        return Ok(value.as_bytes().to_vec());
    }

    let codec = legacy_encoding(encoding).ok_or(Error::UnsupportedEncoding { encoding })?;
    let (encoded, _, had_errors) = codec.encode(value);
    if had_errors {
        return Err(Error::InvalidEncoding { encoding });
    }
    Ok(encoded.into_owned())
}
`)
}
