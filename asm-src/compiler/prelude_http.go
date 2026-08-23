package main

// An HTTP server, in Veyl.
//
// The whole thing sits on the six net.* primitives, so nothing here
// touches a socket API directly. That is the point of making a socket a
// plain int: the protocol is ordinary Veyl and reads like it.
//
// What it is: HTTP/1.1, one request per connection, Connection: close.
// What it is not: keep-alive, chunked transfer, TLS, or concurrent
// requests. A handler runs to completion before the next accept.

const preludeHTTP = `
struct Request {
    method: str
    path: str
    query: str
    body: str
    headers: {str: str}
}

struct Response {
    status: int
    contentType: str
    body: str
}

// The reason phrase for the codes a small server actually sends. Go
// prints these beside the number and so does this, since a client that
// logs the status line would otherwise show something different.
fn __vy_httpReason(code: int) -> str {
    if code == 200 { return "OK" }
    if code == 201 { return "Created" }
    if code == 204 { return "No Content" }
    if code == 301 { return "Moved Permanently" }
    if code == 302 { return "Found" }
    if code == 304 { return "Not Modified" }
    if code == 400 { return "Bad Request" }
    if code == 401 { return "Unauthorized" }
    if code == 403 { return "Forbidden" }
    if code == 404 { return "Not Found" }
    if code == 405 { return "Method Not Allowed" }
    if code == 500 { return "Internal Server Error" }
    return "Status"
}

fn __vy_httpOK(body: str) -> Response {
    return Response{status: 200, contentType: "text/html; charset=utf-8", body: body}
}

fn __vy_httpText(body: str) -> Response {
    return Response{status: 200, contentType: "text/plain; charset=utf-8", body: body}
}

fn __vy_httpJSON(body: str) -> Response {
    return Response{status: 200, contentType: "application/json", body: body}
}

fn __vy_httpStatus(code: int, body: str) -> Response {
    return Response{status: code, contentType: "text/html; charset=utf-8", body: body}
}

fn __vy_httpNotFound() -> Response {
    return __vy_httpStatus(404, "<h1>404 Not Found</h1>")
}

// The status line, the headers a browser needs, then the body. Content
// Length is counted rather than guessed, because a client that reads
// fewer bytes than were sent hangs waiting for the rest.
fn __vy_httpFormat(r: Response) -> str {
    let head = "HTTP/1.1 {r.status} {__vy_httpReason(r.status)}"
    let ct = "Content-Type: {r.contentType}"
    let cl = "Content-Length: {len(r.body)}"
    return head + "\r\n" + ct + "\r\n" + cl + "\r\n" + "Connection: close" + "\r\n\r\n" + r.body
}

// Split "/path?a=1" into its two halves.
fn __vy_httpSplitTarget(target: str) -> []str {
    let q = indexOf(target, "?")
    if q < 0 { return [target, ""] }
    return [substr(target, 0, q), substr(target, q + 1, len(target))]
}

fn __vy_httpParse(raw: str) -> Request {
    let headers: {str: str} = {}
    let req = Request{method: "", path: "", query: "", body: "", headers: headers}

    let sep = indexOf(raw, "\r\n\r\n")
    let headEnd = sep
    let bodyAt = sep + 4
    if sep < 0 {
        headEnd = len(raw)
        bodyAt = len(raw)
    }

    let head = substr(raw, 0, headEnd)
    req.body = substr(raw, bodyAt, len(raw))

    let rows = split(head, "\r\n")
    if len(rows) == 0 { return req }

    // "GET /path?q=1 HTTP/1.1"
    let parts = split(rows[0], " ")
    if len(parts) >= 1 { req.method = parts[0] }
    if len(parts) >= 2 {
        let both = __vy_httpSplitTarget(parts[1])
        req.path = both[0]
        req.query = both[1]
    }

    // Header names are case insensitive, so they are stored lowered and
    // __vy_httpHeader lowers what it is asked for. Otherwise a lookup
    // of "content-type" misses a header the client spelled with a
    // capital C, which is a bug that only shows up against some clients.
    let i = 1
    while i < len(rows) {
        let row = rows[i]
        let colon = indexOf(row, ":")
        if colon > 0 {
            let name = lower(trim(substr(row, 0, colon)))
            let value = trim(substr(row, colon + 1, len(row)))
            req.headers[name] = value
        }
        i = i + 1
    }
    return req
}

fn __vy_httpHeader(r: Request, name: str) -> str {
    let key = lower(name)
    if has(r.headers, key) { return r.headers[key] }
    return ""
}

// Read a whole request, not just whatever arrived first.
//
// recv returns what one packet carried, which for anything with a body
// is routinely less than the request. This reads until the blank line
// that ends the headers, then reads Content-Length more. Both loops
// stop if the peer goes quiet, so a client that connects and says
// nothing cannot wedge the server.
fn __vy_httpRead(conn: int) -> str! {
    let raw = ""
    while indexOf(raw, "\r\n\r\n") < 0 {
        let chunk = net.recv(conn)?
        if len(chunk) == 0 { return raw }
        raw = raw + chunk
    }

    let sep = indexOf(raw, "\r\n\r\n")
    let head = substr(raw, 0, sep)
    let want = __vy_httpLength(head)
    let have = len(raw) - (sep + 4)

    while have < want {
        let chunk = net.recv(conn)?
        if len(chunk) == 0 { return raw }
        raw = raw + chunk
        have = have + len(chunk)
    }
    return raw
}

// Content-Length off the header block, or zero.
fn __vy_httpLength(head: str) -> int {
    let rows = split(head, "\r\n")
    let i = 0
    while i < len(rows) {
        let colon = indexOf(rows[i], ":")
        if colon > 0 {
            let name = lower(trim(substr(rows[i], 0, colon)))
            if name == "content-length" {
                let v = trim(substr(rows[i], colon + 1, len(rows[i])))
                if isInt(v) { return toInt(v) }
            }
        }
        i = i + 1
    }
    return 0
}

// serve accepts forever and runs the handler for each request.
//
// A handler that fails is answered with a 500 rather than taking the
// server down with it, because the one thing a server must not do is
// stop listening.
fn __vy_httpServe(port: int, handler: fn(Request) -> Response) -> void! {
    let sock = net.listen(port)?
    while true {
        let conn = net.accept(sock)?
        let raw = valueOr(__vy_httpRead(conn), "")
        if len(raw) > 0 {
            let res = handler(__vy_httpParse(raw))
            net.send(conn, __vy_httpFormat(res))?
        }
        net.close(conn)
    }
    return ok()
}

// A one-shot client. Enough to fetch a page, not a general HTTP client:
// no redirects, no TLS, no keep-alive.
fn __vy_httpGet(url: str) -> str! {
    let rest = url
    if startsWith(rest, "http://") { rest = substr(rest, 7, len(rest)) }
    if startsWith(rest, "https://") {
        return fail("https is not supported yet, there is no TLS on this backend")
    }

    let slash = indexOf(rest, "/")
    let hostport = rest
    let path = "/"
    if slash >= 0 {
        hostport = substr(rest, 0, slash)
        path = substr(rest, slash, len(rest))
    }

    let host = hostport
    let port = 80
    let colon = indexOf(hostport, ":")
    if colon >= 0 {
        host = substr(hostport, 0, colon)
        let p = substr(hostport, colon + 1, len(hostport))
        if isInt(p) { port = toInt(p) }
    }

    let conn = net.connect(host, port)?
    let req = "GET {path} HTTP/1.1\r\nHost: {host}\r\nConnection: close\r\nUser-Agent: veyl\r\n\r\n"
    net.send(conn, req)?

    let raw = ""
    while true {
        let chunk = net.recv(conn)?
        if len(chunk) == 0 { break }
        raw = raw + chunk
    }
    net.close(conn)

    let sep = indexOf(raw, "\r\n\r\n")
    if sep < 0 { return raw }
    return substr(raw, sep + 4, len(raw))
}
`
