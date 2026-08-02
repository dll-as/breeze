package binding

// FieldError represents a single validation error for a field.
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ValidationError aggregates all field validation errors from a Bind call.
type ValidationError struct {
	Errors []FieldError
}

// Error implements the error interface.
func (v *ValidationError) Error() string {
	if len(v.Errors) == 0 {
		return "validation error"
	}
	if len(v.Errors) == 1 {
		return v.Errors[0].Message
	}
	return "validation error: multiple fields invalid"
}

// ToProblemJSON returns an RFC 9457 problem details JSON object.
// The response status code should be set to 422 Unprocessable Entity by the caller.
func (v *ValidationError) ToProblemJSON() map[string]any {
	errorsList := make([]map[string]string, len(v.Errors))
	for i, fe := range v.Errors {
		errorsList[i] = map[string]string{
			"field":   fe.Field,
			"rule":    fe.Rule,
			"message": fe.Message,
		}
	}

	return map[string]any{
		"type":   "about:blank",
		"title":  "Validation Failed",
		"status": 422,
		"errors": errorsList,
	}
}
