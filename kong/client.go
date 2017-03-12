package kong

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

const (
	apisEndpoint      = "/apis/"
	upstreamsEndpoint = "/upstreams/"
	targetsEndpoint   = "/targets"
)

var (
	// ErrNotFound provides the error when a kong object can't be retrieved.
	ErrNotFound = errors.New("Failed to find the specified kong object")
)

// Client provides a client for interacting
// with the kong API gateway application.
type Client struct {
	host   string
	port   string
	client *http.Client
}

// NewClient creates a new instance
// of the kong client.
func NewClient(host string, port string, scheme string) *Client {
	return &Client{host: scheme + host, port: port, client: http.DefaultClient}
}

// CreateAPI creates a new API in kong.
func (c *Client) CreateAPI(api *API) (*API, error) {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(api)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.host+":"+c.port+apisEndpoint, b)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	var createdAPI *API
	if resp.StatusCode != http.StatusCreated {
		return nil, errors.New("Failed to create the specified API")
	}
	err = json.NewDecoder(resp.Body).Decode(&createdAPI)
	if err != nil {
		return nil, err
	}
	return createdAPI, nil
}

// GetAPI retrieves an API by it's name or id.
func (c *Client) GetAPI(nameOrID string) (*API, error) {
	req, err := http.NewRequest("GET", c.host+":"+c.port+apisEndpoint+nameOrID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	} else if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Failed to retrieve the specified API")
	}
	var api *API
	err = json.NewDecoder(resp.Body).Decode(&api)
	if err != nil {
		return nil, err
	}
	return api, nil
}

// UpdateAPI deals with updating the provided API
// assuming an API exists with the provided ID or name
// if it doesn't exist.
func (c *Client) UpdateAPI(api *API) (*API, error) {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(api)
	if err != nil {
		return nil, err
	}
	var nameOrID string
	if api.ID != "" {
		nameOrID = api.ID
	} else {
		nameOrID = api.Name
	}
	req, err := http.NewRequest("PUT", c.host+":"+c.port+apisEndpoint+nameOrID, b)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, errors.New("Failed to update the specified API")
	}
	var updatedAPI *API
	err = json.NewDecoder(resp.Body).Decode(&updatedAPI)
	if err != nil {
		return nil, err
	}
	return updatedAPI, nil
}

// DeleteAPI deals with removing the specified API.
func (c *Client) DeleteAPI(nameOrID string) error {
	req, err := http.NewRequest("DELETE", c.host+":"+c.port+apisEndpoint+nameOrID, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	} else if resp.StatusCode != http.StatusNoContent {
		return errors.New("Failed to delete the API with the provided identifier")
	}
	return nil
}

// CreateUpstream deals with creating a new upstream object
// which can be referenced by an API as an upstream URL.
func (c *Client) CreateUpstream(upstream *Upstream) (*Upstream, error) {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(upstream)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.host+":"+c.port+upstreamsEndpoint, b)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, errors.New("Failed to create the specified upstream")
	}
	var createdUpstream *Upstream
	err = json.NewDecoder(resp.Body).Decode(&createdUpstream)
	if err != nil {
		return nil, err
	}
	return createdUpstream, nil
}

// GetUpstream deals with retrieving the upstream
// with the specified name or ID.
func (c *Client) GetUpstream(nameOrId string) (*Upstream, error) {
	req, err := http.NewRequest("GET", c.host+":"+c.port+upstreamsEndpoint+nameOrId, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	} else if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Failed to retrieve the specified upstream")
	}
	var upstream *Upstream
	err = json.NewDecoder(resp.Body).Decode(&upstream)
	if err != nil {
		return nil, err
	}
	return upstream, nil
}

// DeleteUpstream deals with removing the upstream
// object with the specified name or ID.
func (c *Client) DeleteUpstream(nameOrId string) error {
	req, err := http.NewRequest("DELETE", c.host+":"+c.port+upstreamsEndpoint+nameOrId, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	} else if resp.StatusCode != http.StatusNoContent {
		return errors.New("Failed to delete the upstream with the provided identifier")
	}
	return nil
}

// UpdateUpstream deals with updating the specified upstream.
func (c *Client) UpdateUpstream(upstream *Upstream) (*Upstream, error) {
	var nameOrId string
	if upstream.ID != "" {
		nameOrId = upstream.ID
	} else {
		nameOrId = upstream.Name
	}
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(upstream)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("PUT", c.host+":"+c.port+apisEndpoint+nameOrId, b)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, errors.New("Failed to update the provided upstream")
	}
	var updatedUpstream *Upstream
	err = json.NewDecoder(resp.Body).Decode(&updatedUpstream)
	if err != nil {
		return nil, err
	}
	return updatedUpstream, nil
}

// CreateTarget deals with adding a new target
// to an existing upstream.
func (c *Client) CreateTarget(upstreamNameOrId string, target *Target) (*Target, error) {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(target)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.host+":"+c.port+upstreamsEndpoint+upstreamNameOrId+targetsEndpoint, b)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, errors.New("Failed to create the specified target for the specified upstream")
	}
	var createdTarget *Target
	err = json.NewDecoder(resp.Body).Decode(&createdTarget)
	if err != nil {
		return nil, err
	}
	return createdTarget, nil
}

// ListTargets lists out all the targets for a specified
// upstream.
func (c *Client) ListTargets(upstreamNameOrId string) (*TargetList, error) {
	req, err := http.NewRequest("GET", c.host+":"+c.port+upstreamsEndpoint+upstreamNameOrId+targetsEndpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	} else if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Failed to retrieve the list of targets for the provided upstream")
	}
	var targetList *TargetList
	err = json.NewDecoder(resp.Body).Decode(&targetList)
	if err != nil {
		return nil, err
	}
	return targetList, nil
}

// DisableTarget creates a new target with the specified host with a weight of 0.
func (c *Client) DisableTarget(upstreamNameOrId string, targetHost string) (*Target, error) {
	return c.newTargetEntry(upstreamNameOrId, targetHost, 0)
}

// EnableTarget creates a new upstream with the weight set to 10 so the load balancer takes
// the upstream target into account. (Upstreams use history for targets so the latest created target gets used)
func (c *Client) EnableTarget(upstreamNameOrId string, targetHost string) (*Target, error) {
	return c.newTargetEntry(upstreamNameOrId, targetHost, 10)
}

// Creates a new kong target object with the provided weight.
func (c *Client) newTargetEntry(upstreamNameOrId string, targetHost string, weight int) (*Target, error) {
	target := &Target{
		Target: targetHost,
		Weight: weight,
	}
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(target)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.host+":"+c.port+upstreamsEndpoint+upstreamNameOrId+targetsEndpoint, b)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	} else if resp.StatusCode != http.StatusCreated {
		return nil, errors.New("Failed to create the new target entry")
	}
	var createdTarget *Target
	err = json.NewDecoder(resp.Body).Decode(&createdTarget)
	if err != nil {
		return nil, err
	}
	return createdTarget, nil
}
