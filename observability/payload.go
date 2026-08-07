package observability

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// maxPayloadChars bounds the rendered payload string. Event payloads are
// arbitrary application structs; without a cap, one fat event could fill
// the ring buffer on its own.
const maxPayloadChars = 512

// describePayload renders an event payload to a short, safe string.
//
// # Why this is not json.Marshal
//
// Marshalling would serialise every exported field, including the ones
// named "Password" or "Token", straight into a buffer the dashboard
// renders. This walks the struct field by field instead and redacts any
// field whose *name* looks sensitive, so a credential never reaches the
// ring buffer in the first place.
//
// Reflection here is deliberate and safe: it runs only when payload
// capture is explicitly enabled, and only once per dispatch — never on
// the default hot path.
func describePayload(v any) string {
	if v == nil {
		return ""
	}
	var sb strings.Builder
	writeValue(&sb, reflect.ValueOf(v), 0)
	s := sb.String()
	if len(s) > maxPayloadChars {
		return s[:maxPayloadChars] + "…"
	}
	return s
}

// maxDepth bounds recursion so a self-referential structure cannot
// produce an unbounded string.
const maxDepth = 3

// writeValue renders one value, redacting sensitive struct fields.
func writeValue(sb *strings.Builder, v reflect.Value, depth int) {
	if depth > maxDepth {
		sb.WriteString("…")
		return
	}
	if !v.IsValid() {
		sb.WriteString("<nil>")
		return
	}

	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			sb.WriteString("<nil>")
			return
		}
		writeValue(sb, v.Elem(), depth)

	case reflect.Struct:
		t := v.Type()
		sb.WriteString(t.Name())
		sb.WriteByte('{')
		first := true
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue // unexported
			}
			if !first {
				sb.WriteString(", ")
			}
			first = false
			sb.WriteString(f.Name)
			sb.WriteByte(':')
			if IsSensitive(f.Name) {
				sb.WriteString(maskedValue)
				continue
			}
			writeValue(sb, v.Field(i), depth+1)
		}
		sb.WriteByte('}')

	case reflect.Slice, reflect.Array:
		// Length only: the contents of a large slice are rarely useful in
		// a live stream and can be arbitrarily big.
		sb.WriteString("[")
		sb.WriteString(strconv.Itoa(v.Len()))
		sb.WriteString(" items]")

	case reflect.Map:
		sb.WriteString("{")
		sb.WriteString(strconv.Itoa(v.Len()))
		sb.WriteString(" keys}")

	case reflect.String:
		s := v.String()
		if len(s) > 64 {
			s = s[:64] + "…"
		}
		sb.WriteString(strconv.Quote(s))

	case reflect.Bool:
		sb.WriteString(strconv.FormatBool(v.Bool()))

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sb.WriteString(strconv.FormatInt(v.Int(), 10))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		sb.WriteString(strconv.FormatUint(v.Uint(), 10))

	case reflect.Float32, reflect.Float64:
		sb.WriteString(strconv.FormatFloat(v.Float(), 'g', -1, 64))

	default:
		fmt.Fprintf(sb, "%v", v.Interface())
	}
}
