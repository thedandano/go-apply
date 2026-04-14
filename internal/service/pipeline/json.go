package pipeline

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// jsonBlockRe matches a JSON object inside a markdown code fence (```json or ```).
var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

// parseJSONFromResponse extracts and parses the first JSON object from an LLM response.
// It handles three common formats:
//  1. Raw JSON: {"key": "value"}
//  2. Markdown-fenced: ```json\n{"key": "value"}\n```
//  3. JSON embedded in prose: "Here is the result: {"key": "value"}"
func parseJSONFromResponse(resp string, v any) error {
	if m := jsonBlockRe.FindStringSubmatch(resp); len(m) > 1 {
		resp = m[1]
	}

	start := -1
	for i, c := range resp {
		if c == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return fmt.Errorf("no JSON object found in response")
	}

	end := -1
	for i := len(resp) - 1; i >= 0; i-- {
		if resp[i] == '}' {
			end = i
			break
		}
	}
	if end == -1 {
		return fmt.Errorf("no closing brace found in response")
	}

	return json.Unmarshal([]byte(resp[start:end+1]), v)
}
