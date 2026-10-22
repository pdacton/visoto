package main

// GET /api/icons.json — the icon set browser-side resolution reads
// (static/js/visoto-icons.js). Base path and defaults are not in it: those ride
// on every page as the #vs-icons island (icon.Config).
//
// It used to be inlined into every Graph Explorer embed as a -available-icons
// island, which grows with the icon set and is repeated on every page with a
// graph. Served once instead, under a URL carrying the set's version
// (icon.ManifestURL), so the browser and the shared cache keep it until a
// deploy changes the set.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/icon"
)

// iconsImmutable is the Cache-Control for a request naming the current version:
// that URL can only ever mean this body.
const iconsImmutable = "public, max-age=31536000, immutable"

// iconsHandler serves the manifest. A missing or stale ?v= (a page cached across
// a deploy) still gets the current set, just not as immutable — otherwise the
// old URL would pin the new body for a year. It bypasses etagMiddleware
// (skipETagPath) because it sets its own Cache-Control.
func iconsHandler(c *gin.Context) {
	r := icon.Current()
	if c.Query("v") == r.Version() {
		c.Header("Cache-Control", iconsImmutable)
	} else {
		c.Header("Cache-Control", "no-cache")
	}
	c.JSON(http.StatusOK, r.Manifest())
}
