package email

import "strings"

// DeriveThreadID picks a stable thread key from RFC822 headers.
//
// Precedence: first Message-ID in References (the original ancestor),
// then In-Reply-To, then the message's own Message-ID. Gmail's threadId
// is intentionally ignored here — it's based on subject heuristics and
// drifts from References, so the inbox UI groups by what RFC822 says.
//
// Returned IDs include the surrounding angle brackets if the source had
// them, normalized via TrimSpace. An empty result means we have no
// usable thread anchor (no headers and no Message-ID) — callers should
// store an empty string so the partial-index predicate skips the row.
func DeriveThreadID(messageID, inReplyTo, references string) string {
	if root := firstReference(references); root != "" {
		return root
	}
	if v := strings.TrimSpace(inReplyTo); v != "" {
		return v
	}
	return strings.TrimSpace(messageID)
}

// firstReference returns the first token in a References header. RFC822
// allows whitespace-separated Message-IDs; we take the first one as the
// thread root (RFC convention is oldest-first).
func firstReference(refs string) string {
	refs = strings.TrimSpace(refs)
	if refs == "" {
		return ""
	}
	for _, f := range strings.Fields(refs) {
		f = strings.TrimSpace(f)
		if f != "" {
			return f
		}
	}
	return ""
}
