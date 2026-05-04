package adminweb

import (
	"net/http"
	"net/url"
	"strings"
)

// Mount returns a handler that serves the admin subtree at basePath and
// delegates everything else to public.
func Mount(basePath string, public http.Handler, admin http.Handler) http.Handler {
	bp := strings.TrimSuffix(strings.TrimSpace(basePath), "/")
	if bp == "" {
		bp = "/admin"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == bp || strings.HasPrefix(r.URL.Path, bp+"/") {
			trim := strings.TrimPrefix(r.URL.Path, bp)
			if trim == "" {
				trim = "/"
			}
			r2 := r.Clone(r.Context())
			u2 := new(url.URL)
			*u2 = *r.URL
			u2.Path = trim
			r2.URL = u2
			admin.ServeHTTP(w, r2)
			return
		}
		public.ServeHTTP(w, r)
	})
}
