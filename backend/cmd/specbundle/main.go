// Command specbundle собирает api/openapi.yaml из многофайлового источника
// (api/openapi.root.yaml + $ref на api/paths/*.yaml, api/components.yaml, api/schemas/*.yaml):
// загружает корень через kin-openapi, поднимает все внешние компоненты в components
// корневого документа (InternalizeRefs) и пишет один файл только с локальными $ref.
//
// Использование: specbundle -o <out> [-o <out2>] <root>
// Несколько -o: один и тот же бандл пишется в несколько мест (api/openapi.yaml и
// копия для //go:embed в backend/internal/openapi).
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

const banner = `# ============================================================
# СГЕНЕРИРОВАННЫЙ ФАЙЛ — не редактируйте напрямую.
# Источник правды: api/openapi.root.yaml + api/paths/*.yaml + api/components.yaml + api/schemas/*.yaml.
# Собирается целью make bundle-api (см. Makefile). Комментарии из источников сюда не попадают.
# ============================================================
`

type outs []string

func (o *outs) String() string     { return strings.Join(*o, ",") }
func (o *outs) Set(s string) error { *o = append(*o, s); return nil }

func main() {
	var out outs
	flag.Var(&out, "o", "выходной файл (можно несколько)")
	flag.Parse()
	if len(out) == 0 || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: specbundle -o <out> [-o <out2>] <root.yaml>")
		os.Exit(2)
	}

	data, err := bundle(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "specbundle: %v\n", err)
		os.Exit(1)
	}
	for _, p := range out {
		if err := writeIfChanged(p, data); err != nil {
			fmt.Fprintf(os.Stderr, "specbundle: %v\n", err)
			os.Exit(1)
		}
	}
}

func bundle(root string) ([]byte, error) {
	loader := &openapi3.Loader{Context: context.Background(), IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(root)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", root, err)
	}

	r := newResolver(doc)
	doc.InternalizeRefs(context.Background(), r.name)
	if err := r.err(); err != nil {
		return nil, err
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	// T.MarshalYAML строит map, и yaml.v3 сортирует ключи по алфавиту: components
	// оказался бы раньше openapi, а пути — не в порядке источника. Порядок путей
	// не косметика: oapi-codegen регистрирует маршруты в порядке строк файла.
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, fmt.Errorf("reparse: %w", err)
	}
	pathOrder, err := rootPathOrder(root)
	if err != nil {
		return nil, err
	}
	top := node.Content[0]
	reorder(top, []string{"openapi", "info", "servers", "tags", "security", "paths", "components"})
	if paths := child(top, "paths"); paths != nil {
		reorder(paths, pathOrder)
	}

	var buf bytes.Buffer
	buf.WriteString(banner)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), nil
}

// rootPathOrder — порядок ключей paths: в корневом файле.
func rootPathOrder(root string) ([]string, error) {
	data, err := os.ReadFile(root)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("parse %s: %w", root, err)
	}
	paths := child(node.Content[0], "paths")
	if paths == nil {
		return nil, fmt.Errorf("%s: нет paths", root)
	}
	keys := make([]string, 0, len(paths.Content)/2)
	for i := 0; i < len(paths.Content); i += 2 {
		keys = append(keys, paths.Content[i].Value)
	}
	return keys, nil
}

// child возвращает значение ключа key в mapping-узле или nil.
func child(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// reorder переставляет пары mapping-узла: сначала ключи из order (в этом порядке),
// затем остальные в прежнем порядке.
func reorder(m *yaml.Node, order []string) {
	rank := make(map[string]int, len(order))
	for i, k := range order {
		rank[k] = i
	}
	type kv struct{ k, v *yaml.Node }
	pairs := make([]kv, 0, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		pairs = append(pairs, kv{m.Content[i], m.Content[i+1]})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		ri, oki := rank[pairs[i].k.Value]
		rj, okj := rank[pairs[j].k.Value]
		switch {
		case oki && okj:
			return ri < rj
		case oki != okj:
			return oki
		default:
			return false
		}
	})
	m.Content = m.Content[:0]
	for _, p := range pairs {
		m.Content = append(m.Content, p.k, p.v)
	}
}

// resolver называет поднятые компоненты последним сегментом фрагмента ссылки
// (…/schemas/Application -> Application). Дефолтный DefaultRefNameResolver склеивает
// путь файла с именем (applications_Application) и переименовал бы все Go-типы.
//
// Цена короткого имени — возможная коллизия: одноимённые компоненты из разных мест
// молча схлопнулись бы в один. Поэтому запоминаем, откуда пришло имя (полная ссылка:
// файл + фрагмент), заранее заносим компоненты, объявленные в самом корне, и падаем,
// если одно имя приходит из двух разных ссылок.
type resolver struct {
	seen       map[string]string // "<collection>/<name>" -> полная ссылка-источник
	collisions []string
	bad        []string // ссылки не вида #/components/<collection>/<Name>
}

// Поддерживаем только ссылки на именованные компоненты: у вложенного фрагмента
// (…/schemas/Outer/properties/status) последний сегмент — не имя компонента.
var componentFragment = regexp.MustCompile(`^/components/[^/]+/[^/]+$`)

func newResolver(doc *openapi3.T) *resolver {
	r := &resolver{seen: map[string]string{}}
	if c := doc.Components; c != nil {
		r.seedRoot("schemas", c.Schemas)
		r.seedRoot("parameters", c.Parameters)
		r.seedRoot("headers", c.Headers)
		r.seedRoot("requestBodies", c.RequestBodies)
		r.seedRoot("responses", c.Responses)
		r.seedRoot("securitySchemes", c.SecuritySchemes)
		r.seedRoot("examples", c.Examples)
		r.seedRoot("links", c.Links)
		r.seedRoot("callbacks", c.Callbacks)
	}
	return r
}

// seedRoot заносит компоненты, объявленные в корне инлайном (Ref == ""): резолвер их
// не видит, а InternalizeRefs молча направил бы одноимённую внешнюю ссылку на них.
func (r *resolver) seedRoot(collection string, m any) {
	rv := reflect.ValueOf(m)
	for _, k := range rv.MapKeys() {
		v := rv.MapIndex(k)
		if v.IsNil() || v.Elem().FieldByName("Ref").String() != "" {
			continue
		}
		r.seen[collection+"/"+k.String()] = "<корень>"
	}
}

func (r *resolver) name(_ *openapi3.T, ref openapi3.ComponentRef) string {
	u := ref.RefPath()
	if u == nil || u.Fragment == "" {
		panic(fmt.Sprintf("specbundle: ссылка без фрагмента: %q", ref.RefString()))
	}
	name := u.Fragment[strings.LastIndex(u.Fragment, "/")+1:]
	if !componentFragment.MatchString(u.Fragment) {
		r.bad = append(r.bad, ref.RefString())
	}
	key := ref.CollectionName() + "/" + name
	src := filepath.ToSlash(u.Path) + "#" + u.Fragment
	if prev, ok := r.seen[key]; ok && prev != src {
		r.collisions = append(r.collisions, fmt.Sprintf("%s: %s и %s", key, prev, src))
	} else {
		r.seen[key] = src
	}
	return name
}

func (r *resolver) err() error {
	var msgs []string
	if len(r.bad) > 0 {
		sort.Strings(r.bad)
		msgs = append(msgs, "ссылки должны указывать на именованный компонент (#/components/<вид>/<Имя>):\n  "+strings.Join(r.bad, "\n  "))
	}
	if len(r.collisions) > 0 {
		sort.Strings(r.collisions)
		msgs = append(msgs, "одно имя компонента приходит из разных мест:\n  "+strings.Join(r.collisions, "\n  "))
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New(strings.Join(msgs, "\n"))
}

// writeIfChanged не трогает идентичный файл: иначе make bundle-api менял бы mtime на каждый запуск.
func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}
