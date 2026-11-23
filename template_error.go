package config

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/infracost/config/template"
)

// TemplateStage is the step of generating a config from a template that failed.
type TemplateStage string

const (
	// TemplateStageSyntax: the template does not parse.
	TemplateStageSyntax TemplateStage = "template_syntax"
	// TemplateStageRun: the template fails while it runs.
	TemplateStageRun TemplateStage = "template_run"
	// TemplateStageYAML: the rendered output is not valid YAML.
	TemplateStageYAML TemplateStage = "yaml"
	// TemplateStageConfig: the rendered output is YAML, but not a valid config.
	TemplateStageConfig TemplateStage = "config"
)

// TemplateError says which step of generating a config from a template failed,
// and where. A line is 0 when unknown.
type TemplateError struct {
	Stage TemplateStage
	// TemplateLine is a line of the template. Set for the template stages.
	TemplateLine int
	// TemplateColumn is the 1-based byte column in TemplateLine where the
	// failing action starts. Set for run errors.
	TemplateColumn int
	// RenderedLine is a line of the rendered output. Set for the yaml and
	// config stages.
	RenderedLine int
	// Message is the failure without its location, the template or the
	// sentinel text.
	Message string

	err error
}

func (e *TemplateError) Error() string {
	if e.err == nil {
		return e.Message
	}
	return e.err.Error()
}

func (e *TemplateError) Unwrap() error { return e.err }

var (
	// templatePlaceRe matches text/template's "template: NAME:LINE[:BYTE]: "
	// prefix, and the `executing "NAME" ` that run errors add after it.
	templatePlaceRe = regexp.MustCompile(`^template: [^:]*:(\d+)(?::(\d+))?: (?:executing "[^"]*" )?`)
	yamlLineRe      = regexp.MustCompile(`\bline (\d+)`)
)

// newTemplateError wraps err, which carries the sentinel and full message.
// cause is the underlying failure, used for Message and the line.
func newTemplateError(stage TemplateStage, err, cause error) *TemplateError {
	msg := trimSentinels(cause.Error())
	te := &TemplateError{Stage: stage, Message: msg, err: err}
	switch stage {
	case TemplateStageSyntax, TemplateStageRun:
		if m := templatePlaceRe.FindStringSubmatch(msg); m != nil {
			te.TemplateLine, _ = strconv.Atoi(m[1])
			if m[2] != "" {
				offset, _ := strconv.Atoi(m[2])
				te.TemplateColumn = offset + 1
			}
			te.Message = msg[len(m[0]):]
		}
	default:
		te.RenderedLine = lineOf(yamlLineRe, msg)
	}
	return te
}

// trimSentinels removes the sentinel prefixes, which nested wraps repeat.
func trimSentinels(msg string) string {
	for {
		trimmed := strings.TrimPrefix(msg, ErrInvalidConfigYAML.Error()+": ")
		trimmed = strings.TrimPrefix(trimmed, ErrInvalidConfigTemplate.Error()+": ")
		if trimmed == msg {
			return msg
		}
		msg = trimmed
	}
}

func lineOf(re *regexp.Regexp, msg string) int {
	m := re.FindStringSubmatch(msg)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// compileError returns the stage and cause of an error from
// template.Parser.Compile.
func compileError(err error) (TemplateStage, error) {
	var ce *template.CompileError
	if !errors.As(err, &ce) {
		return TemplateStageRun, err
	}
	if ce.Step == template.ErrParse {
		return TemplateStageSyntax, ce.Err
	}
	return TemplateStageRun, ce.Err
}
