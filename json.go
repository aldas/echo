// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package echo

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// DefaultJSONSerializer implements JSON encoding using encoding/json.
type DefaultJSONSerializer struct{}

// Serialize converts an interface into a json and writes it to the response.
// You can optionally use the indent parameter to produce pretty JSONs.
func (d DefaultJSONSerializer) Serialize(c *Context, target any, indent string) error {
	enc := json.NewEncoder(c.Response())
	if indent != "" {
		enc.SetIndent("", indent)
	}
	return enc.Encode(target)
}

// Deserialize reads a JSON from a request body and converts it into an interface.
func (d DefaultJSONSerializer) Deserialize(c *Context, target any) error {
	err := json.NewDecoder(c.Request().Body).Decode(target)
	if ute, ok := err.(*json.UnmarshalTypeError); ok {
		return NewHTTPErrorWithInternal(
			http.StatusBadRequest,
			err,
			fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, field=%v, offset=%v", ute.Type, ute.Value, ute.Field, ute.Offset),
		)
	} else if se, ok := err.(*json.SyntaxError); ok {
		return NewHTTPErrorWithInternal(http.StatusBadRequest,
			err,
			fmt.Sprintf("Syntax error: offset=%v, error=%v", se.Offset, se.Error()),
		)
	}
	return err
}
