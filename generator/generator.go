package generator

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/wzshiming/profile_stats"
	"github.com/wzshiming/profile_stats/generator/activities"
	"github.com/wzshiming/profile_stats/generator/charts"
	"github.com/wzshiming/profile_stats/generator/now"
	"github.com/wzshiming/profile_stats/generator/placeholder"
	"github.com/wzshiming/profile_stats/generator/stats"
	"github.com/wzshiming/profile_stats/source"
	"github.com/wzshiming/xmlinjector"
)

const (
	key       = "PROFILE_STATS"
	blankChar = "\n"
)

type Handler struct {
	registry map[string]profile_stats.Generator
}

func NewHandler(src *source.Source) *Handler {
	r := &Handler{
		registry: map[string]profile_stats.Generator{},
	}

	r.register("now", now.NewNow())
	r.register("updatedat", now.NewNow())
	r.register("placeholder", placeholder.NewPlaceHolder())
	r.register("activities", activities.NewActivities(src))
	r.register("stats", stats.NewStats(src))
	r.register("charts", charts.NewCharts(src))
	return r
}

func (r *Handler) register(name string, generator profile_stats.Generator) {
	r.registry[name] = generator
}

func (r *Handler) Handle(ctx context.Context, origin []byte) ([]byte, []string, error) {
	handleStart := time.Now()
	buf := bytes.NewBuffer(nil)
	var warnings []string
	var generatorCount int
	date, err := xmlinjector.Inject([]byte(key), origin, func(args, origin []byte) []byte {
		tag := NewArgs(string(args), true)
		template, ok := tag.String("template")
		if !ok || template == "" {
			warnings = append(warnings, fmt.Sprintf("%q: no template", args))
			return errInfo("no template", origin)
		}

		blank, ok := tag.Int("blank")
		if !ok {
			blank = 2
		}

		generator, ok := r.registry[template]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%q: not support template %q", args, template))
			return errInfo(fmt.Sprintf("not support template %q", template), origin)
		}

		generatorCount++
		genStart := time.Now()
		buf.Reset()
		err := generator.Generate(ctx, buf, tag)
		genDuration := time.Since(genStart)
		if err != nil {
			log.Printf("[generator:%s] failed in %v: %v", template, genDuration, err)
			warnings = append(warnings, fmt.Sprintf("%q: %s", args, err.Error()))
			return errInfo(err.Error(), origin)
		}

		outputSize := buf.Len()
		log.Printf("[generator:%s] completed in %v (output: %d bytes)", template, genDuration, outputSize)

		raw := buf.Bytes()
		raw = bytes.Trim(raw, blankChar)

		var tmp []byte

		if blank > 0 {
			tmp = make([]byte, 0, len(raw)+blank*2)
			blanks := bytes.Repeat([]byte(blankChar), blank)
			tmp = append(tmp, blanks...)
			tmp = append(tmp, raw...)
			tmp = append(tmp, blanks...)
		} else {
			tmp = make([]byte, len(raw))
			copy(tmp, raw)
		}
		return tmp
	})
	handleDuration := time.Since(handleStart)
	if generatorCount > 0 {
		log.Printf("[handler] completed in %v (processed %d generator(s))", handleDuration, generatorCount)
	}
	return date, warnings, err
}

func errInfo(msg string, origin []byte) []byte {
	return append([]byte(fmt.Sprintf("\n<!-- profile_stats_error error:%q date:%q /-->\n", msg, time.Now().Format(time.RFC3339))), origin...)
}
