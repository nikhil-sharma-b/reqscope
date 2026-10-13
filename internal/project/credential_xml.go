package project

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// XML is sanitized by position, including unregistered server-created values.
// Invalid or truncated documents are withheld rather than rendered partially.
func (c *Credentials) sanitizeXML(body string, used []localCredential) (string, bool) {
	decoder := xml.NewDecoder(strings.NewReader(body))
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	hiddenDepth := 0
	sensitive := func(name string) bool {
		if sensitivePosition(name) {
			return true
		}
		for _, item := range used {
			if item.Field != "" && strings.EqualFold(name, item.Field) {
				return true
			}
		}
		return false
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		switch t := token.(type) {
		case xml.StartElement:
			if hiddenDepth > 0 {
				hiddenDepth++
				continue
			}
			for i, attr := range t.Attr {
				if sensitive(attr.Name.Local) {
					t.Attr[i].Value = "[REDACTED]"
				} else {
					t.Attr[i].Value = c.Sanitize(attr.Value)
				}
			}
			token = t
			if sensitive(t.Name.Local) {
				if encoder.EncodeToken(token) != nil || encoder.EncodeToken(xml.CharData("[REDACTED]")) != nil {
					return "", false
				}
				hiddenDepth = 1
				continue
			}
		case xml.EndElement:
			if hiddenDepth > 0 {
				hiddenDepth--
				if hiddenDepth > 0 {
					continue
				}
			}
		case xml.CharData:
			if hiddenDepth > 0 {
				continue
			}
			token = xml.CharData(c.Sanitize(string(t)))
		default:
			if hiddenDepth > 0 {
				continue
			}
		}
		if err := encoder.EncodeToken(token); err != nil {
			return "", false
		}
	}
	if hiddenDepth > 0 || encoder.Flush() != nil {
		return "", false
	}
	return c.Sanitize(buffer.String()), true
}
