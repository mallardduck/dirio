package render

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSON encodes v as a single JSON line to w. Errors are written to errW.
func JSON(w, errW io.Writer, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(errW, "error: json encode: %v\n", err)
		return
	}
	fmt.Fprintln(w, string(data))
}
