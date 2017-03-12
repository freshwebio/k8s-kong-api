package kong

// API represents a subset of the kong API object
// which provide the properties this integration utilises.
type API struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	URIs        []string `json:"uris"`
	UpstreamURL string   `json:"upstream_url"`
	StripURI    bool     `json:"strip_uri,omitempty"`
}

// Upstream provides a subset of the kong Upstream object.
// We only care about the name, maybe in the future it will be worth supporting
// the other properties.
type Upstream struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// Target provides the kong Target object
// to be used in upstreams.
type Target struct {
	ID         string `json:"id,omitempty"`
	Target     string `json:"target"`
	Weight     int    `json:"weight"`
	UpstreamID string `json:"upstream_id,omitempty"`
	Created    int    `json:"created_at,omitempty"`
}

// TargetList provides the data structure
// for a list of upstream targets.
type TargetList struct {
	Total int       `json:"total"`
	Data  []*Target `json:"data"`
}
