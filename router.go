package echo

import (
	"errors"
	"net/http"
	"net/url"
)

// Router is interface for routing requests to registered routes.
type Router interface {
	// Add registers Route with Router
	Add(route Route) error
	// Remove removes route from router
	Remove(method string, path string) error
	// Routes returns all registered routes
	Routes() Routes

	// Match searches Router for matching route and applies it to result fields.
	Match(req *http.Request, params *PathParams) RouteMatch
}

// RouteBuilder is optional interface that Router implementation could implement. RouteBuilder allows (re)building Router
// just before Echo server is started. This allows Router act as factory and return different router instances
// depending on registered routes and their characteristics (i.e. if no param/any routes are registered, Build could return
// instance that has code paths only for static routes and therefore being more efficient).
type RouteBuilder interface {
	Build() (Router, error) // FIXME: implement somewhere or delete
}

// RouteMatch is result object for Router.Match. Its main purpose is to avoid allocating memory for PathParams inside router.
type RouteMatch struct {
	// RoutePath contains original path with what matched route was registered with (including placeholders etc).
	RoutePath string
	// Handler handler chain/function that was matched by router. In case of no match could result to NotFoundHandler or MethodNotAllowedHandler.
	Handler HandlerFunc
}

// PathParams is collections of PathParam instances with various helper methods
type PathParams []PathParam

// PathParam is path parameter name and value tuple
type PathParam struct {
	Name  string
	Value string
}

// DefaultRouter is the registry of all registered routes for an `Echo` instance for
// request matching and URL path parameter parsing.
type DefaultRouter struct {
	tree   *node
	routes Routes
	echo   *Echo

	duplicateRouteOverwritesRoute bool
	unescapePathParamValues       bool
}

type children []*node

type node struct {
	kind           kind
	label          byte
	prefix         string
	parent         *node
	staticChildren children
	originalPath   string
	paramNames     []string
	methodHandler  *methodHandler
	paramChild     *node
	anyChild       *node
	// isLeaf indicates that node does not have child routes
	isLeaf bool
	// isHandler indicates that node has at least one handler registered to it
	isHandler bool
}

type kind uint8

type methodHandler struct {
	connect  HandlerFunc // FIXME: add paramNames for each method type to support different names for param/any parameters
	delete   HandlerFunc
	get      HandlerFunc
	head     HandlerFunc
	options  HandlerFunc
	patch    HandlerFunc
	post     HandlerFunc
	propfind HandlerFunc
	put      HandlerFunc
	trace    HandlerFunc
	report   HandlerFunc
	// FIXME map for any other user given method to support arbitrary methods
}

const (
	staticKind kind = iota
	paramKind
	anyKind

	paramLabel = byte(':')
	anyLabel   = byte('*')
)

func (m *methodHandler) isHandler() bool {
	return m.get != nil ||
		m.post != nil ||
		m.options != nil ||
		m.put != nil ||
		m.delete != nil ||
		m.connect != nil ||
		m.head != nil ||
		m.patch != nil ||
		m.propfind != nil ||
		m.trace != nil ||
		m.report != nil
}

// DefaultRouterOptFunc is option function for DefaultRouter
type DefaultRouterOptFunc func(r *DefaultRouter)

// RouterWithUnescapedPathParamValues instructs DefaultRouter to unescape path parameter value
func RouterWithUnescapedPathParamValues() DefaultRouterOptFunc {
	return func(r *DefaultRouter) {
		r.unescapePathParamValues = true
	}
}

// RouterWithDuplicateRouteOverwritesRoute instructs DefaultRouter not to return error and to overwrite Route when new
// one is registered with same method+path
func RouterWithDuplicateRouteOverwritesRoute() DefaultRouterOptFunc {
	return func(r *DefaultRouter) {
		r.duplicateRouteOverwritesRoute = true
	}
}

// NewRouter returns a new Router instance.
func NewRouter(e *Echo, opts ...DefaultRouterOptFunc) *DefaultRouter {
	r := &DefaultRouter{
		tree: &node{
			methodHandler: new(methodHandler),
		},
		routes: make(Routes, 0),
		echo:   e,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Routes returns all registered routes
func (r *DefaultRouter) Routes() Routes {
	return r.routes
}

// Build is no-op function for DefaultRouter.
func (r *DefaultRouter) Build() (Router, error) {
	return r, nil
}

// Remove unregisters registered route
func (r *DefaultRouter) Remove(method string, path string) error {
	panic("not implemented") // FIXME: implement
}

// AddRouteError is error returned by Router.Add containing information what actual route adding failed. Useful for
// mass adding (i.e. Any() routes)
type AddRouteError struct {
	Method string
	Path   string
	Name   string
	Err    error
}

func (e *AddRouteError) Error() string { return e.Method + " " + e.Path + ": " + e.Err.Error() }

func (e *AddRouteError) Unwrap() error { return e.Err }

func newAddRouteError(route Route, err error) *AddRouteError {
	return &AddRouteError{
		Method: route.Method,
		Path:   route.Path,
		Name:   route.Name,
		Err:    err,
	}
}

// Add registers a new route for method and path with matching handler.
func (r *DefaultRouter) Add(route Route) error {
	if route.Handler == nil {
		return newAddRouteError(route, errors.New("adding route without handler function"))
	}
	method := route.Method
	path := route.Path
	h := applyMiddleware(route.Handler, route.Middlewares...)
	if route.Name == "" {
		route.Name = method + ":" + route.Path
	}
	//if r.duplicateRouteOverwritesRoute
	// FIXME: check if name is unique (unless overwriting same method+path)

	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	pnames := []string{} // Param names
	ppath := path        // Original path
	for i, lcpIndex := 0, len(path); i < lcpIndex; i++ {
		if path[i] == ':' {
			j := i + 1

			r.insert(method, path[:i], nil, staticKind, "", nil)
			for ; i < lcpIndex && path[i] != '/'; i++ {
			}

			pnames = append(pnames, path[j:i])
			path = path[:j] + path[i:]
			i, lcpIndex = j, len(path)

			if i == lcpIndex {
				// path node is last fragment of route path. ie. `/users/:id`
				r.insert(method, path[:i], h, paramKind, ppath, pnames)
			} else {
				r.insert(method, path[:i], nil, paramKind, "", nil)
			}
		} else if path[i] == '*' {
			r.insert(method, path[:i], nil, staticKind, "", nil)
			pnames = append(pnames, "*")
			r.insert(method, path[:i+1], h, anyKind, ppath, pnames)
		}
	}

	// FIXME: check duplicate values in `paramNames` and return error (what about `*`)
	r.insert(method, path, h, staticKind, ppath, pnames)

	r.routes = append(r.routes, route)
	return nil
}

func (r *DefaultRouter) insert(method, path string, h HandlerFunc, t kind, ppath string, pnames []string) {
	currentNode := r.tree // Current node as root
	search := path

	for {
		searchLen := len(search)
		prefixLen := len(currentNode.prefix)
		lcpLen := 0

		// LCP - Longest Common Prefix (https://en.wikipedia.org/wiki/LCP_array)
		max := prefixLen
		if searchLen < max {
			max = searchLen
		}
		for ; lcpLen < max && search[lcpLen] == currentNode.prefix[lcpLen]; lcpLen++ {
		}

		if lcpLen == 0 {
			// At root node
			currentNode.label = search[0]
			currentNode.prefix = search
			if h != nil {
				currentNode.kind = t
				currentNode.addHandler(method, h)
				currentNode.originalPath = ppath
				currentNode.paramNames = pnames
			}
			currentNode.isLeaf = currentNode.staticChildren == nil && currentNode.paramChild == nil && currentNode.anyChild == nil
		} else if lcpLen < prefixLen {
			// Split node
			n := newNode(
				currentNode.kind,
				currentNode.prefix[lcpLen:],
				currentNode,
				currentNode.staticChildren,
				currentNode.methodHandler,
				currentNode.originalPath,
				currentNode.paramNames,
				currentNode.paramChild,
				currentNode.anyChild,
			)
			// Update parent path for all children to new node
			for _, child := range currentNode.staticChildren {
				child.parent = n
			}
			if currentNode.paramChild != nil {
				currentNode.paramChild.parent = n
			}
			if currentNode.anyChild != nil {
				currentNode.anyChild.parent = n
			}

			// Reset parent node
			currentNode.kind = staticKind
			currentNode.label = currentNode.prefix[0]
			currentNode.prefix = currentNode.prefix[:lcpLen]
			currentNode.staticChildren = nil
			currentNode.methodHandler = new(methodHandler)
			currentNode.originalPath = ""
			currentNode.paramNames = nil
			currentNode.paramChild = nil
			currentNode.anyChild = nil
			currentNode.isLeaf = false
			currentNode.isHandler = false

			// Only Static children could reach here
			currentNode.addStaticChild(n)

			if lcpLen == searchLen {
				// At parent node
				currentNode.kind = t
				currentNode.addHandler(method, h)
				currentNode.originalPath = ppath
				currentNode.paramNames = pnames
			} else {
				// Create child node
				n = newNode(t, search[lcpLen:], currentNode, nil, new(methodHandler), ppath, pnames, nil, nil)
				n.addHandler(method, h)
				// Only Static children could reach here
				currentNode.addStaticChild(n)
			}
			currentNode.isLeaf = currentNode.staticChildren == nil && currentNode.paramChild == nil && currentNode.anyChild == nil
		} else if lcpLen < searchLen {
			search = search[lcpLen:]
			c := currentNode.findChildWithLabel(search[0])
			if c != nil {
				// Go deeper
				currentNode = c
				continue
			}
			// Create child node
			n := newNode(t, search, currentNode, nil, new(methodHandler), ppath, pnames, nil, nil)
			n.addHandler(method, h)
			switch t {
			case staticKind:
				currentNode.addStaticChild(n)
			case paramKind:
				currentNode.paramChild = n
			case anyKind:
				currentNode.anyChild = n
			}
			currentNode.isLeaf = currentNode.staticChildren == nil && currentNode.paramChild == nil && currentNode.anyChild == nil
		} else {
			// Node already exists
			if h != nil {
				currentNode.addHandler(method, h)
				currentNode.originalPath = ppath
				if len(currentNode.paramNames) == 0 { // Issue #729
					currentNode.paramNames = pnames
				}
			}
		}
		return
	}
}

func newNode(t kind, pre string, p *node, sc children, mh *methodHandler, ppath string, pnames []string, paramChildren, anyChildren *node) *node {
	return &node{
		kind:           t,
		label:          pre[0],
		prefix:         pre,
		parent:         p,
		staticChildren: sc,
		originalPath:   ppath,
		paramNames:     pnames,
		methodHandler:  mh,
		paramChild:     paramChildren,
		anyChild:       anyChildren,
		isLeaf:         sc == nil && paramChildren == nil && anyChildren == nil,
		isHandler:      mh.isHandler(),
	}
}

func (n *node) addStaticChild(c *node) {
	n.staticChildren = append(n.staticChildren, c)
}

func (n *node) findStaticChild(l byte) *node {
	for _, c := range n.staticChildren {
		if c.label == l {
			return c
		}
	}
	return nil
}

func (n *node) findChildWithLabel(l byte) *node {
	for _, c := range n.staticChildren {
		if c.label == l {
			return c
		}
	}
	if l == paramLabel {
		return n.paramChild
	}
	if l == anyLabel {
		return n.anyChild
	}
	return nil
}

func (n *node) addHandler(method string, h HandlerFunc) {
	switch method {
	case http.MethodConnect:
		n.methodHandler.connect = h
	case http.MethodDelete:
		n.methodHandler.delete = h
	case http.MethodGet:
		n.methodHandler.get = h
	case http.MethodHead:
		n.methodHandler.head = h
	case http.MethodOptions:
		n.methodHandler.options = h
	case http.MethodPatch:
		n.methodHandler.patch = h
	case http.MethodPost:
		n.methodHandler.post = h
	case PROPFIND:
		n.methodHandler.propfind = h
	case http.MethodPut:
		n.methodHandler.put = h
	case http.MethodTrace:
		n.methodHandler.trace = h
	case REPORT:
		n.methodHandler.report = h
	}

	if h != nil {
		n.isHandler = true
	} else {
		n.isHandler = n.methodHandler.isHandler()
	}
}

func (n *node) findHandler(method string) HandlerFunc {
	switch method {
	case http.MethodConnect:
		return n.methodHandler.connect
	case http.MethodDelete:
		return n.methodHandler.delete
	case http.MethodGet:
		return n.methodHandler.get
	case http.MethodHead:
		return n.methodHandler.head
	case http.MethodOptions:
		return n.methodHandler.options
	case http.MethodPatch:
		return n.methodHandler.patch
	case http.MethodPost:
		return n.methodHandler.post
	case PROPFIND:
		return n.methodHandler.propfind
	case http.MethodPut:
		return n.methodHandler.put
	case http.MethodTrace:
		return n.methodHandler.trace
	case REPORT:
		return n.methodHandler.report
	default:
		return nil
	}
}

func (n *node) checkMethodNotAllowed() HandlerFunc {
	for _, m := range methods {
		if h := n.findHandler(m); h != nil {
			return MethodNotAllowedHandler
		}
	}
	return NotFoundHandler
}

// Match looks up a handler registered for method and path. It also parses URL for path
// parameters and load them into context.
//
// For performance:
//
// - Get context from `Echo#AcquireContext()`
// - Reset it `Context#Reset()`
// - Return it `Echo#ReleaseContext()`.
func (r *DefaultRouter) Match(req *http.Request, pathParams *PathParams) RouteMatch {
	result := RouteMatch{
		Handler:   NotFoundHandler,
		RoutePath: "",
	}
	//paramValues := *pathParams // expand to maximum capacity
	//paramValues = paramValues[0:cap(paramValues)]
	*pathParams = (*pathParams)[0:cap(*pathParams)]

	var (
		currentNode           = r.tree // Current node as root
		previousBestMatchNode *node
		matchedHandler        HandlerFunc
		// search stores the remaining path to check for match. By each iteration we move from start of path to end of the path
		// and search value gets shorter and shorter.
		path        = GetPath(req)
		search      = path
		searchIndex = 0
		paramIndex  int // Param counter
	)

	// Backtracking is needed when a dead end (leaf node) is reached in the router tree.
	// To backtrack the current node will be changed to the parent node and the next kind for the
	// router logic will be returned based on fromKind or kind of the dead end node (static > param > any).
	// For example if there is no static node match we should check parent next sibling by kind (param).
	// Backtracking itself does not check if there is a next sibling, this is done by the router logic.
	backtrackToNextNodeKind := func(fromKind kind) (nextNodeKind kind, valid bool) {
		previous := currentNode
		currentNode = previous.parent
		valid = currentNode != nil

		// Next node type by priority
		if previous.kind == anyKind {
			nextNodeKind = staticKind
		} else {
			nextNodeKind = previous.kind + 1
		}

		if fromKind == staticKind {
			// when backtracking is done from static kind block we did not change search so nothing to restore
			return
		}

		// restore search to value it was before we move to current node we are backtracking from.
		if previous.kind == staticKind {
			searchIndex -= len(previous.prefix)
		} else {
			paramIndex--
			// for param/any node.prefix value is always `:` so we can not deduce searchIndex from that and must use pValue
			// for that index as it would also contain part of path we cut off before moving into node we are backtracking from
			searchIndex -= len((*pathParams)[paramIndex].Value)
			(*pathParams)[paramIndex].Value = ""
		}
		search = path[searchIndex:]
		return
	}

	// Router tree is implemented by longest common prefix array (LCP array) https://en.wikipedia.org/wiki/LCP_array
	// Tree search is implemented as for loop where one loop iteration is divided into 3 separate blocks
	// Each of these blocks checks specific kind of node (static/param/any). Order of blocks reflex their priority in routing.
	// Search order/priority is: static > param > any.
	//
	// Note: backtracking in tree is implemented by replacing/switching currentNode to previous node
	// and hoping to (goto statement) next block by priority to check if it is the match.
	for {
		prefixLen := 0 // Prefix length
		lcpLen := 0    // LCP (longest common prefix) length

		if currentNode.kind == staticKind {
			searchLen := len(search)
			prefixLen = len(currentNode.prefix)

			// LCP - Longest Common Prefix (https://en.wikipedia.org/wiki/LCP_array)
			max := prefixLen
			if searchLen < max {
				max = searchLen
			}
			for ; lcpLen < max && search[lcpLen] == currentNode.prefix[lcpLen]; lcpLen++ {
			}
		}

		if lcpLen != prefixLen {
			// No matching prefix, let's backtrack to the first possible alternative node of the decision path
			nk, ok := backtrackToNextNodeKind(staticKind)
			if !ok {
				*pathParams = (*pathParams)[0:0]
				return result // No other possibilities on the decision path
			} else if nk == paramKind {
				goto Param
				// NOTE: this case (backtracking from static node to previous any node) can not happen by current any matching logic. Any node is end of search currently
				//} else if nk == anyKind {
				//	goto Any
			} else {
				// Not found (this should never be possible for static node we are looking currently)
				break
			}
		}

		// The full prefix has matched, remove the prefix from the remaining search
		search = search[lcpLen:]
		searchIndex = searchIndex + lcpLen

		// Finish routing if no remaining search and we are on a node with handler and matching method type
		if search == "" && currentNode.isHandler {
			// check if current node has handler registered for http method we are looking for. we store currentNode as
			// best matching in case we do no find no more routes matching this path+method
			if previousBestMatchNode == nil {
				previousBestMatchNode = currentNode
			}
			if h := currentNode.findHandler(req.Method); h != nil {
				matchedHandler = h
				break
			}
		}

		// Static node
		if search != "" {
			if child := currentNode.findStaticChild(search[0]); child != nil {
				currentNode = child
				continue
			}
		}

	Param:
		// Param node
		if child := currentNode.paramChild; search != "" && child != nil {
			currentNode = child
			i := 0
			l := len(search)
			if currentNode.isLeaf {
				// when param node does not have any children then param node should act similarly to any node - consider all remaining search as match
				i = l
			} else {
				for ; i < l && search[i] != '/'; i++ {
				}
			}

			(*pathParams)[paramIndex].Value = search[:i]
			paramIndex++
			search = search[i:]
			searchIndex = searchIndex + i
			continue
		}

	Any:
		// Any node
		if child := currentNode.anyChild; child != nil {
			// If any node is found, use remaining path for paramValues
			currentNode = child
			(*pathParams)[len(currentNode.paramNames)-1].Value = search
			// update indexes/search in case we need to backtrack when no handler match is found
			paramIndex++
			searchIndex += +len(search)
			search = ""

			// check if current node has handler registered for http method we are looking for. we store currentNode as
			// best matching in case we do no find no more routes matching this path+method
			if previousBestMatchNode == nil {
				previousBestMatchNode = currentNode
			}
			if h := currentNode.findHandler(req.Method); h != nil {
				matchedHandler = h
				break
			}
		}

		// Let's backtrack to the first possible alternative node of the decision path
		nk, ok := backtrackToNextNodeKind(anyKind)
		if !ok {
			break // No other possibilities on the decision path
		} else if nk == paramKind {
			goto Param
		} else if nk == anyKind {
			goto Any
		} else {
			// Not found
			break
		}
	}

	if currentNode == nil && previousBestMatchNode == nil {
		*pathParams = (*pathParams)[0:0]
		return result // nothing matched at all
	}

	if matchedHandler != nil {
		result.Handler = matchedHandler
		result.RoutePath = currentNode.originalPath
	} else {
		// use previous match as basis. although we have no matching handler we have path match.
		// so we can send http.StatusMethodNotAllowed (405) instead of http.StatusNotFound (404)
		currentNode = previousBestMatchNode
		result.Handler = currentNode.checkMethodNotAllowed()
	}

	*pathParams = (*pathParams)[0:len(currentNode.paramNames)]
	for i, name := range currentNode.paramNames {
		(*pathParams)[i].Name = name
	}

	if r.unescapePathParamValues && currentNode.kind != staticKind {
		// See issue #1531, #1258 - there are cases when path parameter need to be unescaped
		for i, p := range *pathParams {
			tmpVal, err := url.PathUnescape(p.Value)
			if err == nil { // handle problems by ignoring them.
				(*pathParams)[i].Value = tmpVal
			}
		}
	}

	return result
}

// Get returns path parameter value for given name or default value.
func (p PathParams) Get(name string, defaultValue string) string {
	for _, param := range p {
		if param.Name == name {
			return param.Value
		}
	}
	return defaultValue
}
