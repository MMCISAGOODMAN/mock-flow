package matcher

import (
	"strings"
)

type Result struct {
	Params      map[string]string
	ResourceKey string
	Score       int
}

func Normalize(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func Match(pattern, requestPath string) (Result, bool) {
	pattern = Normalize(pattern)
	requestPath = Normalize(requestPath)
	pSegs := split(pattern)
	rSegs := split(requestPath)

	params := map[string]string{}
	score := 0
	pi, ri := 0, 0
	for pi < len(pSegs) {
		ps := pSegs[pi]
		if ps == "*" || ps == "{*}" {
			rest := strings.Join(rSegs[ri:], "/")
			if rest != "" {
				params["*"] = rest
			}
			score += 1
			ri = len(rSegs)
			pi++
			if pi != len(pSegs) {
				return Result{}, false
			}
			break
		}
		if ri >= len(rSegs) {
			return Result{}, false
		}
		rs := rSegs[ri]
		if strings.HasPrefix(ps, "{") && strings.HasSuffix(ps, "}") && len(ps) > 2 {
			name := ps[1 : len(ps)-1]
			params[name] = rs
			score += 10
			pi++
			ri++
			continue
		}
		if ps != rs {
			return Result{}, false
		}
		score += 100
		pi++
		ri++
	}
	if ri != len(rSegs) {
		return Result{}, false
	}

	var keys []string
	for _, ps := range pSegs {
		if strings.HasPrefix(ps, "{") && strings.HasSuffix(ps, "}") && len(ps) > 2 {
			name := ps[1 : len(ps)-1]
			if name == "*" {
				continue
			}
			if v, ok := params[name]; ok {
				keys = append(keys, v)
			}
		}
	}
	resourceKey := "_"
	if len(keys) > 0 {
		resourceKey = strings.Join(keys, "/")
	}
	return Result{Params: params, ResourceKey: resourceKey, Score: score}, true
}

func split(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
