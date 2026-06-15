package allenbrain

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the Allen Brain Atlas as a kit Domain: a driver that a
// multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/allenbrain-cli/allenbrain"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then
// dereferences allenbrain:// URIs by routing to the operations Register
// installs. The same Domain also builds the standalone allenbrain binary
// (see cli.NewApp), so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the Allen Brain Atlas driver. It carries no state; the per-run
// client is built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "allenbrain",
		Hosts:  []string{Host, "mouse.brain-map.org", "brain-map.org"},
		Identity: kit.Identity{
			Binary: "allenbrain",
			Short:  "Browse Allen Brain Atlas genes, atlases, and datasets",
			Long: `allenbrain reads public Allen Brain Atlas data over HTTP, shapes it into
clean records, and prints output that pipes into the rest of your tools.
No API key required.`,
			Site: "brain-map.org",
			Repo: "https://github.com/tamnd/allenbrain-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// List ops: paginated collections.
	kit.Handle(app, kit.OpMeta{Name: "genes", Group: "read", List: true,
		Summary: "List genes from the Allen Brain Atlas", URIType: "gene"},
		listGenes)

	kit.Handle(app, kit.OpMeta{Name: "atlases", Group: "read", List: true,
		Summary: "List all reference atlases", URIType: "atlas"},
		listAtlases)

	kit.Handle(app, kit.OpMeta{Name: "datasets", Group: "read", List: true,
		Summary: "List section datasets (Mouse)", URIType: "dataset"},
		listDatasets)

	// Search op: name/acronym fuzzy search.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read",
		Summary: "Search genes by name or acronym",
		Args:    []kit.Arg{{Name: "query", Help: "search term"}}},
		searchGenes)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type genesIn struct {
	Limit  int     `kit:"flag,inherit" help:"max results" default:"25"`
	Start  int     `kit:"flag" help:"start row offset"`
	Client *Client `kit:"inject"`
}

type datasetsIn struct {
	Limit  int     `kit:"flag,inherit" help:"max results" default:"25"`
	Start  int     `kit:"flag" help:"start row offset"`
	Client *Client `kit:"inject"`
}

type searchIn struct {
	Query  string  `kit:"arg" help:"search term"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"25"`
	Client *Client `kit:"inject"`
}

type atlasesIn struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listGenes(ctx context.Context, in genesIn, emit func(Gene) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	genes, err := in.Client.Genes(ctx, limit, in.Start)
	if err != nil {
		return mapErr(err)
	}
	for _, g := range genes {
		if err := emit(g); err != nil {
			return err
		}
	}
	return nil
}

func listAtlases(ctx context.Context, in atlasesIn, emit func(Atlas) error) error {
	atlases, err := in.Client.Atlases(ctx)
	if err != nil {
		return mapErr(err)
	}
	for _, a := range atlases {
		if err := emit(a); err != nil {
			return err
		}
	}
	return nil
}

func listDatasets(ctx context.Context, in datasetsIn, emit func(Dataset) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	datasets, err := in.Client.Datasets(ctx, limit, in.Start)
	if err != nil {
		return mapErr(err)
	}
	for _, d := range datasets {
		if err := emit(d); err != nil {
			return err
		}
	}
	return nil
}

func searchGenes(ctx context.Context, in searchIn, emit func(Gene) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	genes, err := in.Client.SearchGenes(ctx, in.Query, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, g := range genes {
		if err := emit(g); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id) pair,
// so `ant resolve` and `ant url` touch no network.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	// Strip scheme prefixes.
	for _, pfx := range []string{"allenbrain://gene/", "allenbrain://atlas/", "allenbrain://dataset/"} {
		if strings.HasPrefix(input, pfx) {
			typ := strings.TrimPrefix(pfx, "allenbrain://")
			typ = strings.TrimSuffix(typ, "/")
			return typ, strings.TrimPrefix(input, pfx), nil
		}
	}
	// Bare numeric id — classify as gene for backwards compat.
	if isAllDigits(input) {
		return "gene", input, nil
	}
	return "", "", errs.Usage("unrecognized Allen Brain reference: %q", input)
}

// Locate is the inverse: the live human-facing URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "gene":
		return fmt.Sprintf("%s/gene/show/%s", BrainURL, id), nil
	case "atlas":
		return fmt.Sprintf("%s/atlas/show/%s", BrainURL, id), nil
	case "dataset":
		return fmt.Sprintf("%s/experiment/show/%s", BrainURL, id), nil
	default:
		return "", errs.Usage("allenbrain has no resource type %q", uriType)
	}
}

// --- helpers ---

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// geneID extracts the bare numeric id from any accepted gene reference.
func geneID(s string) string {
	s = strings.TrimSpace(s)
	if _, after, ok := strings.Cut(s, "/gene/show/"); ok {
		return strings.TrimSpace(after)
	}
	// Try to parse as a plain number.
	if _, err := strconv.Atoi(s); err == nil {
		return s
	}
	return s
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}

// suppress unused warning on geneID
var _ = geneID
