package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/opengfx1030/bernie/internal/support/redact"
)

// spec describes one tool. The catalog is fixed: names, descriptions and
// schemas never depend on run logs, recipes or any other host data.
type spec struct {
	name, desc string
}

func (sp spec) annotations() *sdk.ToolAnnotations {
	f := false
	return &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &f, OpenWorldHint: &f}
}

// schemaFor infers the JSON Schema of T, the canonical Go type of a document.
func schemaFor[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](&jsonschema.ForOptions{})
	if err != nil {
		panic(fmt.Sprintf("mcp: schema of %T: %v", *new(T), err))
	}
	return s
}

// handler is a tool body. It returns the canonical document of its command.
type handler[In any] func(ctx context.Context, c *call, in In) (any, error)

// addTool registers a tool. In is the input type (its schema is inferred and
// then refined by tweak); Out is the canonical output type, used only for
// the output schema. The result is redacted before it leaves the process.
func addTool[In, Out any](s *Server, sp spec, tweak func(*jsonschema.Schema), h handler[In]) {
	in := schemaFor[In]()
	if tweak != nil {
		tweak(in)
	}
	t := &sdk.Tool{Name: sp.name, Description: sp.desc, InputSchema: in, OutputSchema: schemaFor[Out](), Annotations: sp.annotations()}
	sdk.AddTool(s.srv, t, func(ctx context.Context, req *sdk.CallToolRequest, args In) (*sdk.CallToolResult, any, error) {
		c := &call{s: s, name: sp.name, req: req, ctx: ctx, start: time.Now()}
		out, err := h(ctx, c, args)
		if err == nil {
			out, err = redact.JSON(out)
		}
		c.finish(args, err)
		return nil, out, err
	})
}

// call is the state of one tool call, for the audit record and progress.
type call struct {
	s     *Server
	name  string
	req   *sdk.CallToolRequest
	ctx   context.Context
	start time.Time

	// Set by the mutation flow.
	result  string // planned, applied, rejected, failed; empty: ok or failed
	class   string // error class
	digest  string
	tokenID string
}

// finish writes the audit record. A failed audit write goes to the log; it
// never changes the result of the call.
func (c *call) finish(args any, err error) {
	e := auditEntry{
		Time: c.start.UTC(), Transport: c.s.o.Transport, Tool: c.name,
		Digest: c.digest, TokenID: c.tokenID, Result: c.result, ErrorClass: c.class,
		DurationMS: time.Since(c.start).Milliseconds(),
	}
	if ip := c.req.Session.InitializeParams(); ip != nil && ip.ClientInfo != nil {
		e.Client = ip.ClientInfo.Name
	}
	if e.Result == "" {
		e.Result = "ok"
		if err != nil {
			e.Result, e.ErrorClass = "failed", "error"
		}
	}
	e.Args = argSummary(args)
	if werr := c.s.audit.write(e); werr != nil {
		fmt.Fprintf(c.s.o.Log, "bernie mcp: audit: %v\n", werr)
	}
}

// argSummary is the redacted arguments, without the confirm token, cut to 512 bytes.
func argSummary(args any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	delete(m, "confirm_token")
	g, err := redact.JSON(m)
	if err != nil {
		return ""
	}
	b, _ = json.Marshal(g)
	if len(b) > 512 {
		b = append(b[:512:512], "..."...)
	}
	return string(b)
}

// progress returns a writer that sends each line written to it as a
// notifications/progress message, when the client asked for progress. Else it
// discards. Lines are redacted.
func (c *call) progress() io.Writer {
	tok := c.req.Params.GetProgressToken()
	if tok == nil {
		return io.Discard
	}
	return &progressWriter{c: c, token: tok}
}

type progressWriter struct {
	c     *call
	token any
	n     float64
	buf   []byte
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimSpace(p.buf[:i]))
		p.buf = p.buf[i+1:]
		if line == "" {
			continue
		}
		p.n++
		_ = p.c.req.Session.NotifyProgress(p.c.ctx, &sdk.ProgressNotificationParams{
			ProgressToken: p.token, Progress: p.n, Message: redact.Text(line),
		})
	}
	return len(b), nil
}
