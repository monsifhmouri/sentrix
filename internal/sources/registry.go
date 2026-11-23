package sources

import "fmt"

type factory struct {
	info Info
	make func(Settings) Source
}

var registry = []factory{
	{Info{"crtsh", "Certificate Transparency log search (hostnames from issued certificates)", AccessPublic, ""}, newCrtsh},
	{Info{"wayback", "Internet Archive CDX API (historical URLs captured by the Wayback Machine)", AccessPublic, ""}, newWayback},
	{Info{"commoncrawl", "Common Crawl index (URLs from the most recent crawl)", AccessPublic, ""}, newCommonCrawl},
	{Info{"urlscan", "urlscan.io public search (pages previously scanned by anyone)", AccessOptionalKey, "SENTRIX_URLSCAN_API_KEY"}, newURLScan},
	{Info{"otx", "AlienVault OTX (URLs and passive DNS hostnames associated with the domain)", AccessOptionalKey, "SENTRIX_OTX_API_KEY"}, newOTX},
}

// Catalog lists every built-in source without touching the network.
func Catalog() []Info {
	out := make([]Info, len(registry))
	for i, f := range registry {
		out[i] = f.info
	}
	return out
}

// Build constructs the named source.
func Build(name string, s Settings) (Source, error) {
	for _, f := range registry {
		if f.info.Name == name {
			return f.make(s), nil
		}
	}
	return nil, fmt.Errorf("unknown source %q", name)
}
