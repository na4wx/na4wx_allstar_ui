package server

import (
	"net/http"

	"hamvoipconfiggui/internal/config"
)

// nodeListEntry is one row of the Nodes list page: identity plus
// whatever the node directory knows about it, deliberately without any
// of the live asterisk -rx calls Home/Stats make — this page is for
// finding and managing a node (edit, delete, add), not watching it, so
// it stays fast and cheap to load regardless of how many nodes are
// configured or whether Asterisk is even running.
type nodeListEntry struct {
	Node     *config.Node
	Callsign string
}

type nodesIndexPageData struct {
	pageData
	Nodes []nodeListEntry
}

// handleNodesIndex lists every configured node with a quick link to its
// full edit page — the fast, no-CLI-calls counterpart to Home (live
// status) and Stats (detailed history), for simply finding/adding/
// removing a node.
func (s *Server) handleNodesIndex(w http.ResponseWriter, r *http.Request) {
	numbers, err := s.store.ListNodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var entries []nodeListEntry
	for _, n := range numbers {
		node, err := s.store.LoadNode(n)
		if err != nil {
			continue // skip malformed entries rather than failing the whole page
		}
		entries = append(entries, nodeListEntry{Node: node, Callsign: s.nodes.Label(n)})
	}
	s.render(w, "nodes_index.html", nodesIndexPageData{
		pageData: pageData{LoggedIn: true},
		Nodes:    entries,
	})
}
