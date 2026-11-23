package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"sentrix/pkg/models"
)

type otx struct{ base }

func newOTX(s Settings) Source { return &otx{newBase("https://otx.alienvault.com", s)} }

func (o *otx) Name() string   { return "otx" }
func (o *otx) Access() Access { return AccessOptionalKey }
func (o *otx) Description() string {
	return "AlienVault OTX (URLs and passive DNS hostnames associated with the domain)"
}

func (o *otx) headers() map[string]string {
	if o.apiKey == "" {
		return nil
	}
	return map[string]string{"X-OTX-API-KEY": o.apiKey}
}

func (o *otx) Available(ctx context.Context) error {
	return o.ping(ctx, o.url+"/api/v1/indicators/domain/example.com/general", o.headers())
}

type otxURLList struct {
	URLList []struct {
		URL      string `json:"url"`
		Date     string `json:"date"`
		HTTPCode int    `json:"httpcode"`
	} `json:"url_list"`
	HasNext bool `json:"has_next"`
}

type otxPassiveDNS struct {
	PassiveDNS []struct {
		Hostname   string `json:"hostname"`
		Address    string `json:"address"`
		RecordType string `json:"record_type"`
		First      string `json:"first"`
		Last       string `json:"last"`
	} `json:"passive_dns"`
}

const otxLayout = "2006-01-02T15:04:05"

func (o *otx) Fetch(ctx context.Context, domain string) ([]Finding, error) {
	var out []Finding
	var errs []error

	const pageSize = 500
	for page := 1; len(out) < o.max; page++ {
		u := fmt.Sprintf("%s/api/v1/indicators/domain/%s/url_list?limit=%d&page=%d", o.url, url.PathEscape(domain), pageSize, page)
		body, err := o.get(ctx, u, o.headers())
		if err != nil {
			errs = append(errs, err)
			break
		}
		var list otxURLList
		if err := json.Unmarshal(body, &list); err != nil {
			errs = append(errs, fmt.Errorf("%w: otx url_list: %v", ErrParse, err))
			break
		}
		for _, e := range list.URLList {
			out = append(out, Finding{
				Raw:        e.URL,
				Kind:       models.KindEndpoint,
				Evidence:   fmt.Sprintf("OTX url_list entry, HTTP code %d", e.HTTPCode),
				ObservedAt: parseTimestamp(otxLayout, e.Date),
			})
		}
		if !list.HasNext {
			break
		}
	}

	body, err := o.get(ctx, fmt.Sprintf("%s/api/v1/indicators/domain/%s/passive_dns", o.url, url.PathEscape(domain)), o.headers())
	if err != nil {
		errs = append(errs, err)
		return out, errors.Join(errs...)
	}
	var dns otxPassiveDNS
	if err := json.Unmarshal(body, &dns); err != nil {
		errs = append(errs, fmt.Errorf("%w: otx passive_dns: %v", ErrParse, err))
		return out, errors.Join(errs...)
	}
	for _, r := range dns.PassiveDNS {
		out = append(out, Finding{
			Raw:        r.Hostname,
			Kind:       models.KindHost,
			Evidence:   fmt.Sprintf("OTX passive DNS %s -> %s, first %s, last %s", r.RecordType, r.Address, r.First, r.Last),
			ObservedAt: parseTimestamp(otxLayout, r.First),
		})
	}
	return out, errors.Join(errs...)
}
