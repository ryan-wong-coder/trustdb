package pkcs11signer

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxURIBytes = 4096

// TokenSelector contains the RFC 7512 token attributes supported by the
// adapter. At least label or serial must be fixed by the process config.
type TokenSelector struct {
	Label        string
	Manufacturer string
	Model        string
	Serial       string
}

// ObjectSelector contains an exact private-key object selection. A label, an
// ID, or both may be present; both are used when supplied.
type ObjectSelector struct {
	Token TokenSelector
	Label string
	ID    []byte
}

func parseTokenURI(raw string) (TokenSelector, error) {
	attributes, err := parseURIAttributes(raw)
	if err != nil {
		return TokenSelector{}, err
	}
	for name := range attributes {
		switch name {
		case "token", "manufacturer", "model", "serial":
		default:
			return TokenSelector{}, errors.New("PKCS#11 token URI contains an object or unsupported attribute")
		}
	}
	selector, err := tokenSelector(attributes)
	if err != nil {
		return TokenSelector{}, err
	}
	if selector.Label == "" && selector.Serial == "" {
		return TokenSelector{}, errors.New("PKCS#11 token URI must contain token or serial")
	}
	return selector, nil
}

func parseObjectURI(raw string) (ObjectSelector, error) {
	attributes, err := parseURIAttributes(raw)
	if err != nil {
		return ObjectSelector{}, err
	}
	for name := range attributes {
		switch name {
		case "token", "manufacturer", "model", "serial", "object", "id", "type":
		default:
			return ObjectSelector{}, errors.New("PKCS#11 key URI contains an unsupported attribute")
		}
	}
	if objectType := attributes["type"]; objectType != nil && string(objectType) != "private" {
		return ObjectSelector{}, errors.New("PKCS#11 key URI type must be private")
	}
	label := attributes["object"]
	id := attributes["id"]
	if len(label) == 0 && len(id) == 0 {
		return ObjectSelector{}, errors.New("PKCS#11 key URI must contain object or id")
	}
	token, err := tokenSelector(attributes)
	if err != nil {
		return ObjectSelector{}, err
	}
	return ObjectSelector{
		Token: token,
		Label: string(label),
		ID:    append([]byte(nil), id...),
	}, nil
}

func parseURIAttributes(raw string) (map[string][]byte, error) {
	if raw == "" || len(raw) > maxURIBytes || strings.ContainsAny(raw, "\r\n\t\x00") {
		return nil, errors.New("PKCS#11 URI is empty or malformed")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "pkcs11" || parsed.Opaque == "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.String() != raw {
		return nil, errors.New("PKCS#11 URI is malformed")
	}
	items := strings.Split(parsed.Opaque, ";")
	names := make([]string, 0, len(items))
	attributes := make(map[string][]byte, len(items))
	for _, item := range items {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || parts[1] == "" || !validAttributeName(parts[0]) {
			return nil, errors.New("PKCS#11 URI contains a malformed attribute")
		}
		if _, exists := attributes[parts[0]]; exists {
			return nil, errors.New("PKCS#11 URI contains a duplicate attribute")
		}
		if parts[0] == "pin-value" || parts[0] == "pin-source" {
			return nil, errors.New("PKCS#11 URI must not contain PIN material")
		}
		value, err := url.PathUnescape(parts[1])
		if err != nil || value == "" || len(value) > maxURIBytes || strings.IndexByte(value, 0) >= 0 {
			return nil, errors.New("PKCS#11 URI contains an invalid attribute value")
		}
		if parts[0] != "id" && !validTextValue(value) {
			return nil, errors.New("PKCS#11 URI contains an invalid text attribute")
		}
		attributes[parts[0]] = []byte(value)
		names = append(names, parts[0])
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if !equalStrings(names, sorted) {
		return nil, errors.New("PKCS#11 URI attributes must be sorted")
	}
	return attributes, nil
}

func tokenSelector(attributes map[string][]byte) (TokenSelector, error) {
	selector := TokenSelector{
		Label:        string(attributes["token"]),
		Manufacturer: string(attributes["manufacturer"]),
		Model:        string(attributes["model"]),
		Serial:       string(attributes["serial"]),
	}
	for _, value := range []string{selector.Label, selector.Manufacturer, selector.Model, selector.Serial} {
		if value != "" && !validTextValue(value) {
			return TokenSelector{}, errors.New("PKCS#11 token selector is invalid")
		}
	}
	return selector, nil
}

func (s TokenSelector) matchesIdentity(identity TokenIdentity) bool {
	return (s.Label == "" || s.Label == identity.Label) &&
		(s.Manufacturer == "" || s.Manufacturer == identity.Manufacturer) &&
		(s.Model == "" || s.Model == identity.Model) &&
		(s.Serial == "" || s.Serial == identity.Serial)
}

func (s TokenSelector) permits(key TokenSelector) bool {
	return (key.Label == "" || key.Label == s.Label) &&
		(key.Manufacturer == "" || key.Manufacturer == s.Manufacturer) &&
		(key.Model == "" || key.Model == s.Model) &&
		(key.Serial == "" || key.Serial == s.Serial)
}

func (s ObjectSelector) cacheKey() string {
	return fmt.Sprintf("%s\x00%x", s.Label, s.ID)
}

func validAttributeName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validTextValue(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalTokenIdentity(a, b TokenIdentity) bool {
	return a == b
}

func equalObjectSelector(a, b ObjectSelector) bool {
	return a.Label == b.Label && bytes.Equal(a.ID, b.ID) && a.Token == b.Token
}
