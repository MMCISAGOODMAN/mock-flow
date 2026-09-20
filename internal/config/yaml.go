package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"mockflow/internal/store"

	"gopkg.in/yaml.v3"
)

type Document struct {
	Version   int              `yaml:"version"`
	Projects  []ProjectConfig  `yaml:"projects,omitempty"`
	Endpoints []EndpointConfig `yaml:"endpoints,omitempty"`
}

type ProjectConfig struct {
	Name        string           `yaml:"name"`
	Port        int              `yaml:"port"`
	Description string           `yaml:"description,omitempty"`
	Endpoints   []EndpointConfig `yaml:"endpoints"`
}

type EndpointConfig struct {
	Method      string        `yaml:"method"`
	Path        string        `yaml:"path"`
	Description string        `yaml:"description,omitempty"`
	Stages      []StageConfig `yaml:"stages"`
}

type StageConfig struct {
	AfterSeconds int            `yaml:"after_seconds"`
	StatusCode   int            `yaml:"status_code"`
	Headers      map[string]any `yaml:"headers,omitempty"`
	Body         any            `yaml:"body,omitempty"`
}

func Export(projects []store.Project) ([]byte, error) {
	doc := Document{Version: 2}
	for _, p := range projects {
		pc := ProjectConfig{
			Name:        p.Name,
			Port:        p.Port,
			Description: p.Description,
		}
		for _, e := range p.Endpoints {
			pc.Endpoints = append(pc.Endpoints, endpointToConfig(e))
		}
		doc.Projects = append(doc.Projects, pc)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

func endpointToConfig(e store.Endpoint) EndpointConfig {
	ec := EndpointConfig{Method: e.Method, Path: e.Path, Description: e.Description}
	for _, st := range e.Stages {
		sc := StageConfig{AfterSeconds: st.AfterSeconds, StatusCode: st.StatusCode}
		if strings.TrimSpace(st.Headers) != "" && st.Headers != "{}" {
			var h map[string]any
			if err := json.Unmarshal([]byte(st.Headers), &h); err == nil {
				sc.Headers = h
			}
		}
		if strings.TrimSpace(st.Body) != "" {
			var body any
			if err := json.Unmarshal([]byte(st.Body), &body); err == nil {
				sc.Body = body
			} else {
				sc.Body = st.Body
			}
		}
		ec.Stages = append(ec.Stages, sc)
	}
	return ec
}

func Import(data []byte) ([]store.Project, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("line 1: empty document")
	}
	docNode := root.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: document must be a mapping", docNode.Line)
	}
	fields := mapping(docNode)
	verNode, ok := fields["version"]
	if !ok {
		return nil, fmt.Errorf("line %d: missing version", docNode.Line)
	}
	switch verNode.Value {
	case "1":
		eps, err := parseEndpointList(fields, docNode, "endpoints")
		if err != nil {
			return nil, err
		}
		return []store.Project{{Name: "Default", Port: 8080, Description: "默认项目", Endpoints: eps}}, nil
	case "2":
		return parseV2(fields, docNode)
	default:
		return nil, fmt.Errorf("line %d: unsupported version %s (expected 1 or 2)", verNode.Line, verNode.Value)
	}
}

func parseV2(fields map[string]*yaml.Node, docNode *yaml.Node) ([]store.Project, error) {
	psNode, ok := fields["projects"]
	if !ok {
		return nil, fmt.Errorf("line %d: missing projects", docNode.Line)
	}
	if psNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("line %d: projects must be a list", psNode.Line)
	}
	seenPort := map[int]bool{}
	var out []store.Project
	for i, n := range psNode.Content {
		if n.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d: projects[%d] must be a mapping", n.Line, i)
		}
		f := mapping(n)
		name := "Project"
		if nn, ok := f["name"]; ok && strings.TrimSpace(nn.Value) != "" {
			name = strings.TrimSpace(nn.Value)
		}
		port := 8080
		if pn, ok := f["port"]; ok {
			if _, err := fmt.Sscanf(pn.Value, "%d", &port); err != nil {
				return nil, fmt.Errorf("line %d: projects[%d].port must be an integer", pn.Line, i)
			}
		}
		if !store.ValidPort(port) {
			return nil, fmt.Errorf("line %d: projects[%d].port must be 1024-65535", n.Line, i)
		}
		if seenPort[port] {
			return nil, fmt.Errorf("line %d: duplicate project port %d", n.Line, port)
		}
		seenPort[port] = true
		desc := ""
		if d, ok := f["description"]; ok {
			desc = d.Value
		}
		eps, err := parseEndpointList(f, n, fmt.Sprintf("projects[%d].endpoints", i))
		if err != nil {
			return nil, err
		}
		out = append(out, store.Project{Name: name, Port: port, Description: desc, Endpoints: eps})
	}
	return out, nil
}

func parseEndpointList(fields map[string]*yaml.Node, parent *yaml.Node, label string) ([]store.Endpoint, error) {
	epsNode, ok := fields["endpoints"]
	if !ok {
		if strings.HasPrefix(label, "projects[") {
			return nil, nil
		}
		return nil, fmt.Errorf("line %d: missing %s", parent.Line, label)
	}
	if epsNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("line %d: %s must be a list", epsNode.Line, label)
	}
	var out []store.Endpoint
	for i, n := range epsNode.Content {
		ep, err := parseEndpoint(n, i)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, nil
}

func parseEndpoint(n *yaml.Node, idx int) (store.Endpoint, error) {
	if n.Kind != yaml.MappingNode {
		return store.Endpoint{}, fmt.Errorf("line %d: endpoints[%d] must be a mapping", n.Line, idx)
	}
	f := mapping(n)
	methodN, ok := f["method"]
	if !ok || strings.TrimSpace(methodN.Value) == "" {
		return store.Endpoint{}, fmt.Errorf("line %d: endpoints[%d].method is required", n.Line, idx)
	}
	method := strings.ToUpper(strings.TrimSpace(methodN.Value))
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		return store.Endpoint{}, fmt.Errorf("line %d: endpoints[%d].method is invalid", methodN.Line, idx)
	}
	pathN, ok := f["path"]
	if !ok || strings.TrimSpace(pathN.Value) == "" {
		line := n.Line
		if ok {
			line = pathN.Line
		}
		return store.Endpoint{}, fmt.Errorf("line %d: endpoints[%d].path is required", line, idx)
	}
	path := strings.TrimSpace(pathN.Value)
	if !strings.HasPrefix(path, "/") {
		return store.Endpoint{}, fmt.Errorf("line %d: endpoints[%d].path must start with /", pathN.Line, idx)
	}
	desc := ""
	if d, ok := f["description"]; ok {
		desc = d.Value
	}
	var stages []store.Stage
	if sn, ok := f["stages"]; ok {
		if sn.Kind != yaml.SequenceNode {
			return store.Endpoint{}, fmt.Errorf("line %d: endpoints[%d].stages must be a list", sn.Line, idx)
		}
		for j, stn := range sn.Content {
			st, err := parseStage(stn, idx, j)
			if err != nil {
				return store.Endpoint{}, err
			}
			stages = append(stages, st)
		}
	}
	return store.Endpoint{Method: method, Path: path, Description: desc, Stages: stages}, nil
}

func parseStage(n *yaml.Node, ei, si int) (store.Stage, error) {
	if n.Kind != yaml.MappingNode {
		return store.Stage{}, fmt.Errorf("line %d: endpoints[%d].stages[%d] must be a mapping", n.Line, ei, si)
	}
	f := mapping(n)
	after := 0
	if an, ok := f["after_seconds"]; ok {
		if _, err := fmt.Sscanf(an.Value, "%d", &after); err != nil {
			return store.Stage{}, fmt.Errorf("line %d: endpoints[%d].stages[%d].after_seconds must be an integer", an.Line, ei, si)
		}
		if after < 0 {
			return store.Stage{}, fmt.Errorf("line %d: endpoints[%d].stages[%d].after_seconds must be >= 0", an.Line, ei, si)
		}
	}
	status := 200
	if sn, ok := f["status_code"]; ok {
		if _, err := fmt.Sscanf(sn.Value, "%d", &status); err != nil {
			return store.Stage{}, fmt.Errorf("line %d: endpoints[%d].stages[%d].status_code must be an integer", sn.Line, ei, si)
		}
		if status < 100 || status > 599 {
			return store.Stage{}, fmt.Errorf("line %d: endpoints[%d].stages[%d].status_code is invalid", sn.Line, ei, si)
		}
	}
	headers := "{}"
	if hn, ok := f["headers"]; ok {
		raw, err := nodeToJSON(hn)
		if err != nil {
			return store.Stage{}, fmt.Errorf("line %d: endpoints[%d].stages[%d].headers: %w", hn.Line, ei, si, err)
		}
		headers = string(raw)
	}
	body := ""
	if bn, ok := f["body"]; ok {
		raw, err := nodeToJSON(bn)
		if err != nil {
			return store.Stage{}, fmt.Errorf("line %d: endpoints[%d].stages[%d].body: %w", bn.Line, ei, si, err)
		}
		body = string(raw)
	}
	return store.Stage{AfterSeconds: after, StatusCode: status, Headers: headers, Body: body}, nil
}

func nodeToJSON(n *yaml.Node) ([]byte, error) {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func mapping(n *yaml.Node) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		out[k.Value] = v
	}
	return out
}
