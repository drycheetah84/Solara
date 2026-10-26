package main

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// Placeholders shared with tools/difftest-oracle:
//
//	${secret:NAME} -> value from the file named by SOLARA_SECRETS_FILE
//	${var:NAME}    -> value captured from an earlier response ("capture": {"NAME": "a.b.c"})
var (
	vars       = map[string]string{}
	secretVals map[string]string
	phRe       = regexp.MustCompile(`\$\{(secret|var):([A-Za-z0-9_]+)\}`)
)

func secretsMap() map[string]string {
	if secretVals != nil {
		return secretVals
	}
	secretVals = map[string]string{}
	f, err := os.Open(os.Getenv("SOLARA_SECRETS_FILE"))
	if err != nil {
		return secretVals
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if k, v, ok := strings.Cut(t, "="); ok {
			secretVals[strings.TrimSpace(k)] = v
		}
	}
	return secretVals
}

// applySubst substitutes inside a raw JSON step (values are JSON-string escaped).
func applySubst(raw []byte) []byte {
	return phRe.ReplaceAllFunc(raw, func(m []byte) []byte {
		g := phRe.FindSubmatch(m)
		var v string
		if string(g[1]) == "secret" {
			v = secretsMap()[string(g[2])]
		} else {
			v = vars[string(g[2])]
		}
		b, _ := json.Marshal(v)
		return b[1 : len(b)-1]
	})
}

func capture(spec map[string]string, body string) {
	var root any
	if json.Unmarshal([]byte(body), &root) != nil {
		return
	}
	for name, path := range spec {
		cur, ok := root, true
		for _, seg := range strings.Split(path, ".") {
			m, isObj := cur.(map[string]any)
			if !isObj {
				ok = false
				break
			}
			if cur, ok = m[seg]; !ok {
				break
			}
		}
		if !ok {
			continue
		}
		if s, isStr := cur.(string); isStr {
			vars[name] = s
		} else {
			b, _ := json.Marshal(cur)
			vars[name] = string(b)
		}
	}
}
