package allenbrain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL
	return c
}

func geneJSON(id int, acronym, name string) map[string]any {
	return map[string]any{"id": id, "acronym": acronym, "name": name, "entrez_id": id}
}

func atlasJSON(id int, name, imageType string) map[string]any {
	return map[string]any{"id": id, "name": name, "image_type": imageType}
}

func datasetJSON(id, planeSection int, genes []map[string]any) map[string]any {
	return map[string]any{
		"id":                    id,
		"plane_of_section_id":   planeSection,
		"reference_space_id":    9,
		"genes":                 genes,
	}
}

func writeJSON(w http.ResponseWriter, msg any) {
	resp := map[string]any{
		"success":    true,
		"id":         0,
		"start_row":  0,
		"num_rows":   1,
		"total_rows": 1,
		"msg":        msg,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func TestGetSetsUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(`{"success":true,"total_rows":0,"msg":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"total_rows":0,"msg":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.Retries = 5

	start := time.Now()
	_, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGenes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/Gene/query.json" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		writeJSON(w, []map[string]any{
			geneJSON(1, "A1BG", "alpha-1-B glycoprotein"),
			geneJSON(2, "A2M", "alpha-2-macroglobulin"),
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	genes, err := c.Genes(context.Background(), 25, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(genes) != 2 {
		t.Fatalf("got %d genes, want 2", len(genes))
	}
	if genes[0].Acronym != "A1BG" {
		t.Errorf("genes[0].Acronym = %q, want A1BG", genes[0].Acronym)
	}
	if genes[1].Name != "alpha-2-macroglobulin" {
		t.Errorf("genes[1].Name = %q, want alpha-2-macroglobulin", genes[1].Name)
	}
}

func TestSearchGenes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		criteria := r.URL.Query().Get("criteria")
		if criteria == "" {
			t.Error("search request missing criteria parameter")
		}
		writeJSON(w, []map[string]any{
			geneJSON(42, "BRCA1", "BRCA1 DNA repair associated"),
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	genes, err := c.SearchGenes(context.Background(), "BRCA", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(genes) != 1 {
		t.Fatalf("got %d genes, want 1", len(genes))
	}
	if genes[0].Acronym != "BRCA1" {
		t.Errorf("genes[0].Acronym = %q, want BRCA1", genes[0].Acronym)
	}
}

func TestAtlases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/Atlas/query.json" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		writeJSON(w, []map[string]any{
			atlasJSON(1, "Mouse, P56, Coronal", "Coronal"),
			atlasJSON(2, "Mouse, P56, Sagittal", "Sagittal"),
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	atlases, err := c.Atlases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(atlases) != 2 {
		t.Fatalf("got %d atlases, want 2", len(atlases))
	}
	if atlases[0].Name != "Mouse, P56, Coronal" {
		t.Errorf("atlases[0].Name = %q", atlases[0].Name)
	}
	if atlases[0].ImageType != "Coronal" {
		t.Errorf("atlases[0].ImageType = %q, want Coronal", atlases[0].ImageType)
	}
}

func TestDatasets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/SectionDataSet/query.json" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("include") != "genes" {
			t.Error("datasets request missing include=genes")
		}
		writeJSON(w, []map[string]any{
			datasetJSON(100, 1, []map[string]any{
				geneJSON(1, "A1BG", "alpha-1-B glycoprotein"),
			}),
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	datasets, err := c.Datasets(context.Background(), 25, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 1 {
		t.Fatalf("got %d datasets, want 1", len(datasets))
	}
	if datasets[0].ID != 100 {
		t.Errorf("datasets[0].ID = %d, want 100", datasets[0].ID)
	}
	if len(datasets[0].Genes) != 1 {
		t.Fatalf("got %d genes in dataset, want 1", len(datasets[0].Genes))
	}
	if datasets[0].Genes[0].Acronym != "A1BG" {
		t.Errorf("gene.Acronym = %q, want A1BG", datasets[0].Genes[0].Acronym)
	}
}

func TestGetNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
}
