// Plain-language explanations for HTTP error statuses, so a failed fetch tells
// the reader what went wrong instead of a bare "SPARQL request failed: 503".
//
// The statuses that reach the browser come from three places: the SPARQL
// endpoint (passed through by the /api/sparql proxy), the proxy itself (502
// when it cannot reach the endpoint, 400 for a disallowed query type) and
// Caddy in front of Visoto (429 rate limit, 502 when Visoto is down, 504).
//
// Loaded after i18n.js, which provides window.vsT.
(function () {
    'use strict';

    function describe(status) {
        switch (status) {
            case 400: return vsT('js.http.400', 'The query was rejected: a syntax error, or a query type that is not allowed.');
            case 401:
            case 403: return vsT('js.http.403', 'The endpoint refused access.');
            case 404: return vsT('js.http.404', 'The endpoint was not found.');
            case 413: return vsT('js.http.413', 'The query is too large.');
            case 429: return vsT('js.http.429', 'Too many requests in a short time. Wait a minute and try again.');
            case 500: return vsT('js.http.500', 'The endpoint failed while running the query.');
            case 502: return vsT('js.http.502', 'The endpoint, or the Visoto server, could not be reached.');
            case 503: return vsT('js.http.503', 'The endpoint is temporarily unavailable – it may be overloaded or under maintenance. Try again shortly.');
            case 504: return vsT('js.http.504', 'The query timed out. Try a smaller query, or try again later.');
            default: return vsT('js.http.other', 'The request failed.');
        }
    }

    /**
     * Builds the Error to throw for a non-OK response: its message is the
     * explanation followed by the status, e.g. "The query timed out. … (HTTP 504)",
     * and its .status carries the code for callers that branch on it.
     *
     * @param {number} status - The response's HTTP status
     * @returns {Error}
     */
    window.vsHttpError = function (status) {
        var err = new Error(describe(status) + ' (HTTP ' + status + ')');
        err.status = status;
        return err;
    };
})();
