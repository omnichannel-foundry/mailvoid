---
title: MIME parsing
weight: 60
---

# MIME parsing

MailVoid uses the standard library for RFC 5322 headers and a small recursive
MIME walker in `internal/mailparse`. The goal is practical inspection, not a
complete MIME implementation.

## Headers and subject

`net/mail.ReadMessage` parses the header block. Header names are canonicalized
and preserved as `map[string][]string`; the exact block is kept in
`raw_headers`. Only the subject is decoded:

- RFC 2047 encoded words are decoded.
- UTF-8 and US-ASCII are passed through.
- ISO-8859-1/Latin-1 and Windows-1252 are converted with `x/text/charmap`.
- Unknown charsets are left as their original bytes.
- If decoding fails, the original subject string is used.

`raw_headers` is the exact header block, split at the first blank line.

## Body extraction

The walker handles `multipart/*` recursively:

1. Decode the part's `Content-Transfer-Encoding`.
2. If the media type is `multipart/*`, parse with its boundary and recurse.
3. If `text/plain` and no text body has been captured yet, decode its charset
   and store it as `body_text`.
4. If `text/html` and no HTML body has been captured yet, decode its charset and
   store it as `body_html`.
5. Ignore other media types.

The first text part and the first HTML part win. Subsequent alternatives are
ignored for body display but remain in the raw message.

### Transfer encodings

| Encoding | Handling |
|----------|----------|
| `base64` | Whitespace stripped, standard base64 decoded |
| `quoted-printable` | Decoded with `mime/quotedprintable` |
| `7bit`, `8bit`, `binary`, absent | Passed through |

### Charsets

| Charset | Handling |
|---------|----------|
| absent, `utf-8`, `us-ascii`, `ascii` | Byte-for-byte string |
| `iso-8859-1`, `latin1`, `latin-1` | `charmap.ISO8859_1` |
| `windows-1252`, `cp1252` | `charmap.Windows1252` |
| anything else | Byte-for-byte string |

HTML `<meta charset>` declarations are not inspected; only the MIME parameter is
used.

## Attachments

Parts with a `Content-Disposition` beginning with `attachment` are skipped
entirely, including text attachments. The raw message still contains them.

## What is not implemented

- Recursive parsing of `message/rfc822` parts.
- Decoding of non-body headers such as `From` display names or `To` lists.
- Charset detection from HTML metadata.
- MIME parameter continuations beyond what `mime.ParseMediaType` handles.
- HTML sanitization. The console isolates HTML in a sandboxed iframe; the API
  returns the HTML unchanged.

## Failure handling

If header parsing fails:

- the message is still stored;
- `subject` becomes `(unparsable message)`;
- `body_text` and `body_html` are empty;
- an `smtp.error` event with `stored:true` is emitted.

If an individual body part fails to decode, MailVoid falls back to the part's
raw bytes and continues with the remaining parts. The raw message is always
preserved for kept messages.

## Security implications

- HTML is never inserted directly into the MailVoid origin. It is rendered in an
  iframe with `sandbox` and without `allow-scripts` or `allow-same-origin`.
- Plain text and raw messages are HTML-escaped by `html/template`.
- Remote images and styles inside an HTML email may still load inside the
  sandboxed frame. If that is a concern, isolate MailVoid from the network or
  inspect the raw message instead of opening the HTML preview.
