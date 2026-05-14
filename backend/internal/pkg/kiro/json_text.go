package kiro

import "encoding/json"

// JSONText stores Kiro stream input chunks as text. Kiro may emit input either
// as a JSON string delta or as a structured JSON value.
type JSONText string

func (t *JSONText) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*t = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = JSONText(s)
		return nil
	}
	*t = JSONText(string(data))
	return nil
}
