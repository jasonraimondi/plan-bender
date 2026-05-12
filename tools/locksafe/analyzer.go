// Package locksafe provides a Go analyzer that rejects remote I/O while a
// planrepo session lock is live.
package locksafe

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const (
	allowDirective = "//locksafe:allow"
	planrepoSuffix = "/internal/planrepo"
	backendSuffix  = "/internal/backend"
	dispatchSuffix = "/internal/dispatch"
)

// Analyzer reports calls to remote I/O and subprocess APIs while a
// *planrepo.PlanSession is live, and reports PlanSession type containment
// outside the planrepo package.
var Analyzer = &analysis.Analyzer{
	Name: "locksafe",
	Doc:  "reports remote I/O or subprocess execution while a planrepo session lock is live",
	Run:  run,
}

var httpPackageFuncs = map[string]bool{
	"Get":      true,
	"Head":     true,
	"Post":     true,
	"PostForm": true,
}

var httpClientMethods = map[string]bool{
	"Do":       true,
	"Get":      true,
	"Head":     true,
	"Post":     true,
	"PostForm": true,
}

var httpRoundTripperMethods = map[string]bool{
	"RoundTrip": true,
}

var execCmdMethods = map[string]bool{
	"CombinedOutput": true,
	"Output":         true,
	"Run":            true,
	"Start":          true,
	"Wait":           true,
}

type checker struct {
	pass  *analysis.Pass
	allow allowLines
}

type allowLines map[string]map[int]bool

func run(pass *analysis.Pass) (any, error) {
	c := checker{pass: pass, allow: collectAllowLines(pass.Fset, pass.Files)}
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				c.checkFuncType(n.Type)
				if n.Body != nil {
					c.checkFunctionBody(n.Type, n.Body)
				}
			case *ast.FuncLit:
				c.checkFuncType(n.Type)
				if n.Body != nil {
					c.checkFunctionBody(n.Type, n.Body)
				}
				return false
			case *ast.TypeSpec:
				c.checkTypeSpec(n)
			}
			return true
		})
	}
	return nil, nil
}

func collectAllowLines(fset *token.FileSet, files []*ast.File) allowLines {
	out := allowLines{}
	for _, file := range files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if !strings.HasPrefix(comment.Text, allowDirective) {
					continue
				}
				pos := fset.Position(comment.Slash)
				if out[pos.Filename] == nil {
					out[pos.Filename] = map[int]bool{}
				}
				out[pos.Filename][pos.Line] = true
			}
		}
	}
	return out
}

func (c checker) suppressed(pos token.Pos) bool {
	p := c.pass.Fset.Position(pos)
	lines := c.allow[p.Filename]
	return lines[p.Line] || lines[p.Line-1]
}

func (c checker) checkFuncType(fn *ast.FuncType) {
	if c.inPlanrepoPackage() || fn == nil {
		return
	}
	if fn.Params != nil {
		for _, field := range fn.Params.List {
			c.reportPlanSessionType(field.Type, "function parameter")
		}
	}
	if fn.Results != nil {
		for _, field := range fn.Results.List {
			c.reportPlanSessionType(field.Type, "function return")
		}
	}
}

func (c checker) checkTypeSpec(spec *ast.TypeSpec) {
	if c.inPlanrepoPackage() {
		return
	}
	st, ok := spec.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return
	}
	for _, field := range st.Fields.List {
		c.reportPlanSessionType(field.Type, "struct field")
	}
}

func (c checker) reportPlanSessionType(expr ast.Expr, kind string) {
	if expr == nil || c.suppressed(expr.Pos()) {
		return
	}
	typ := c.pass.TypesInfo.TypeOf(expr)
	if !containsPlanSession(typ, map[types.Type]bool{}) {
		return
	}
	c.pass.Reportf(expr.Pos(), "locksafe: %s type recursively contains *planrepo.PlanSession outside internal/planrepo; keep PlanSession contained in planrepo or add //locksafe:allow <reason>", kind)
}

func (c checker) inPlanrepoPackage() bool {
	return strings.HasSuffix(c.pass.Pkg.Path(), planrepoSuffix)
}

func (c checker) checkFunctionBody(fn *ast.FuncType, body *ast.BlockStmt) {
	next := 1
	state := liveState{
		varGroups: map[*types.Var]groupSet{},
		live:      map[int]bool{},
		names:     map[int]string{},
		next:      &next,
	}
	if fn != nil && fn.Params != nil {
		for _, field := range fn.Params.List {
			for _, name := range field.Names {
				if v, ok := objectForIdent(c.pass, name).(*types.Var); ok && isPlanSessionType(v.Type()) {
					state.assignNew(v, name.Name)
				}
			}
		}
	}
	c.processStmtList(body.List, &state)
}

type groupSet map[int]bool

type liveState struct {
	varGroups map[*types.Var]groupSet
	live      map[int]bool
	names     map[int]string
	next      *int
}

func (s *liveState) clone() *liveState {
	out := &liveState{
		varGroups: make(map[*types.Var]groupSet, len(s.varGroups)),
		live:      make(map[int]bool, len(s.live)),
		names:     make(map[int]string, len(s.names)),
		next:      s.next,
	}
	for v, groups := range s.varGroups {
		out.varGroups[v] = copyGroupSet(groups)
	}
	for id, live := range s.live {
		out.live[id] = live
	}
	for id, name := range s.names {
		out.names[id] = name
	}
	return out
}

func (s *liveState) assignNew(v *types.Var, name string) {
	id := *s.next
	*s.next = id + 1
	s.varGroups[v] = groupSet{id: true}
	s.live[id] = true
	s.names[id] = name
}

func (s *liveState) assignGroups(v *types.Var, groups groupSet) {
	if len(groups) == 0 {
		delete(s.varGroups, v)
		return
	}
	s.varGroups[v] = copyGroupSet(groups)
	for id := range groups {
		if _, ok := s.live[id]; !ok {
			s.live[id] = true
		}
	}
}

func (s *liveState) closeVar(v *types.Var) {
	for id := range s.varGroups[v] {
		s.live[id] = false
	}
}

func (s *liveState) liveNames() []string {
	var names []string
	for id, live := range s.live {
		if live {
			names = append(names, s.names[id])
		}
	}
	sort.Strings(names)
	return names
}

func (s *liveState) hasLiveSession() bool {
	for _, live := range s.live {
		if live {
			return true
		}
	}
	return false
}

func copyGroupSet(in groupSet) groupSet {
	out := make(groupSet, len(in))
	for id := range in {
		out[id] = true
	}
	return out
}

func mergeStates(states ...*liveState) *liveState {
	if len(states) == 0 {
		return nil
	}
	out := &liveState{
		varGroups: map[*types.Var]groupSet{},
		live:      map[int]bool{},
		names:     map[int]string{},
		next:      states[0].next,
	}
	for _, state := range states {
		for v, groups := range state.varGroups {
			if out.varGroups[v] == nil {
				out.varGroups[v] = groupSet{}
			}
			for id := range groups {
				out.varGroups[v][id] = true
			}
		}
		for id, live := range state.live {
			out.live[id] = out.live[id] || live
		}
		for id, name := range state.names {
			out.names[id] = name
		}
	}
	return out
}

func (c checker) processStmtList(stmts []ast.Stmt, state *liveState) *liveState {
	for _, stmt := range stmts {
		state = c.processStmt(stmt, state)
	}
	return state
}

func (c checker) processStmt(stmt ast.Stmt, state *liveState) *liveState {
	switch stmt := stmt.(type) {
	case *ast.DeclStmt:
		if decl, ok := stmt.Decl.(*ast.GenDecl); ok {
			for _, spec := range decl.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					state = c.processValueSpec(vs, state)
				}
			}
		}
	case *ast.AssignStmt:
		c.processExprs(stmt.Rhs, state)
		for _, expr := range stmt.Rhs {
			c.applyCloseCalls(expr, state)
		}
		c.applyAssignments(stmt.Lhs, stmt.Rhs, state)
	case *ast.ExprStmt:
		c.processExpr(stmt.X, state)
		c.applyCloseCalls(stmt.X, state)
	case *ast.DeferStmt:
		c.processExpr(stmt.Call, state)
	case *ast.GoStmt:
		c.processExpr(stmt.Call, state)
	case *ast.ReturnStmt:
		c.processExprs(stmt.Results, state)
		for _, expr := range stmt.Results {
			c.applyCloseCalls(expr, state)
		}
	case *ast.BlockStmt:
		state = c.processStmtList(stmt.List, state)
	case *ast.IfStmt:
		state = c.processIfStmt(stmt, state)
	case *ast.ForStmt:
		state = c.processForStmt(stmt, state)
	case *ast.RangeStmt:
		state = c.processRangeStmt(stmt, state)
	case *ast.SwitchStmt:
		state = c.processSwitchStmt(stmt, state)
	case *ast.TypeSwitchStmt:
		state = c.processTypeSwitchStmt(stmt, state)
	case *ast.SelectStmt:
		state = c.processSelectStmt(stmt, state)
	case *ast.LabeledStmt:
		state = c.processStmt(stmt.Stmt, state)
	case *ast.SendStmt:
		c.processExpr(stmt.Chan, state)
		c.processExpr(stmt.Value, state)
	}
	return state
}

func (c checker) processValueSpec(spec *ast.ValueSpec, state *liveState) *liveState {
	c.processExprs(spec.Values, state)
	for _, expr := range spec.Values {
		c.applyCloseCalls(expr, state)
	}
	lhs := make([]ast.Expr, 0, len(spec.Names))
	for _, name := range spec.Names {
		lhs = append(lhs, name)
	}
	c.applyAssignments(lhs, spec.Values, state)
	return state
}

func (c checker) processIfStmt(stmt *ast.IfStmt, state *liveState) *liveState {
	if stmt.Init != nil {
		state = c.processStmt(stmt.Init, state)
	}
	if stmt.Cond != nil {
		c.processExpr(stmt.Cond, state)
		c.applyCloseCalls(stmt.Cond, state)
	}
	bodyState := c.processStmtList(stmt.Body.List, state.clone())
	elseState := state.clone()
	if stmt.Else != nil {
		elseState = c.processStmt(stmt.Else, elseState)
	}
	return mergeStates(bodyState, elseState)
}

func (c checker) processForStmt(stmt *ast.ForStmt, state *liveState) *liveState {
	if stmt.Init != nil {
		state = c.processStmt(stmt.Init, state)
	}
	if stmt.Cond != nil {
		c.processExpr(stmt.Cond, state)
		c.applyCloseCalls(stmt.Cond, state)
	}
	beforeBody := state.clone()
	bodyState := c.processStmtList(stmt.Body.List, state.clone())
	if stmt.Post != nil {
		bodyState = c.processStmt(stmt.Post, bodyState)
	}
	return mergeStates(beforeBody, bodyState)
}

func (c checker) processRangeStmt(stmt *ast.RangeStmt, state *liveState) *liveState {
	c.processExpr(stmt.X, state)
	c.applyCloseCalls(stmt.X, state)
	beforeBody := state.clone()
	bodyState := c.processStmtList(stmt.Body.List, state.clone())
	return mergeStates(beforeBody, bodyState)
}

func (c checker) processSwitchStmt(stmt *ast.SwitchStmt, state *liveState) *liveState {
	if stmt.Init != nil {
		state = c.processStmt(stmt.Init, state)
	}
	if stmt.Tag != nil {
		c.processExpr(stmt.Tag, state)
		c.applyCloseCalls(stmt.Tag, state)
	}
	return c.processCaseClauses(stmt.Body.List, state)
}

func (c checker) processTypeSwitchStmt(stmt *ast.TypeSwitchStmt, state *liveState) *liveState {
	if stmt.Init != nil {
		state = c.processStmt(stmt.Init, state)
	}
	if stmt.Assign != nil {
		state = c.processStmt(stmt.Assign, state)
	}
	return c.processCaseClauses(stmt.Body.List, state)
}

func (c checker) processSelectStmt(stmt *ast.SelectStmt, state *liveState) *liveState {
	branches := make([]*liveState, 0, len(stmt.Body.List)+1)
	hasDefault := false
	for _, clauseNode := range stmt.Body.List {
		clause, ok := clauseNode.(*ast.CommClause)
		if !ok {
			continue
		}
		branch := state.clone()
		if clause.Comm == nil {
			hasDefault = true
		} else {
			branch = c.processStmt(clause.Comm, branch)
		}
		branches = append(branches, c.processStmtList(clause.Body, branch))
	}
	if !hasDefault {
		branches = append(branches, state.clone())
	}
	return mergeStates(branches...)
}

func (c checker) processCaseClauses(clauses []ast.Stmt, state *liveState) *liveState {
	branches := make([]*liveState, 0, len(clauses)+1)
	hasDefault := false
	for _, clauseNode := range clauses {
		clause, ok := clauseNode.(*ast.CaseClause)
		if !ok {
			continue
		}
		branch := state.clone()
		if len(clause.List) == 0 {
			hasDefault = true
		}
		c.processExprs(clause.List, branch)
		branches = append(branches, c.processStmtList(clause.Body, branch))
	}
	if !hasDefault {
		branches = append(branches, state.clone())
	}
	return mergeStates(branches...)
}

func (c checker) processExprs(exprs []ast.Expr, state *liveState) {
	for _, expr := range exprs {
		c.processExpr(expr, state)
	}
}

func (c checker) processExpr(expr ast.Expr, state *liveState) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n := n.(type) {
		case nil:
			return true
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			c.reportRemoteCall(n, state)
		}
		return true
	})
}

func (c checker) applyCloseCalls(expr ast.Expr, state *liveState) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n := n.(type) {
		case nil:
			return true
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if v := c.closeReceiver(n, state); v != nil {
				state.closeVar(v)
			}
		}
		return true
	})
}

func (c checker) closeReceiver(call *ast.CallExpr, state *liveState) *types.Var {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Close" {
		return nil
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}
	v, ok := objectForIdent(c.pass, ident).(*types.Var)
	if !ok || len(state.varGroups[v]) == 0 {
		return nil
	}
	return v
}

func (c checker) reportRemoteCall(call *ast.CallExpr, state *liveState) {
	if !state.hasLiveSession() || c.suppressed(call.Pos()) {
		return
	}
	kind, ok := c.remoteCallKind(call)
	if !ok {
		return
	}
	names := state.liveNames()
	name := "<unknown>"
	if len(names) > 0 {
		name = names[0]
	}
	c.pass.Reportf(call.Pos(), "locksafe: PlanSession %s is live across %s call %s; close it before the call or add //locksafe:allow <reason>", name, kind, formatNode(c.pass.Fset, call.Fun))
}

func (c checker) remoteCallKind(call *ast.CallExpr) (string, bool) {
	if c.isBackendCall(call) {
		return "backend", true
	}
	if c.isNetHTTPRemoteIO(call) {
		return "net/http", true
	}
	if c.isOSExecSubprocess(call) || c.isDispatchSubprocessCall(call) {
		return "subprocess", true
	}
	return "", false
}

func (c checker) isBackendCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := c.pass.TypesInfo.Selections[sel]
	return selection != nil && isBackendType(selection.Recv())
}

func (c checker) isNetHTTPRemoteIO(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if pkgIdent, ok := sel.X.(*ast.Ident); ok {
		if pkg, ok := c.pass.TypesInfo.Uses[pkgIdent].(*types.PkgName); ok {
			return pkg.Imported().Path() == "net/http" && httpPackageFuncs[sel.Sel.Name]
		}
	}
	selection := c.pass.TypesInfo.Selections[sel]
	if selection == nil {
		return false
	}
	return isHTTPClientType(selection.Recv()) && httpClientMethods[sel.Sel.Name] ||
		isHTTPRoundTripperType(selection.Recv()) && httpRoundTripperMethods[sel.Sel.Name]
}

func (c checker) isOSExecSubprocess(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := c.pass.TypesInfo.Selections[sel]
	return selection != nil && isExecCmdType(selection.Recv()) && execCmdMethods[sel.Sel.Name]
}

func (c checker) isDispatchSubprocessCall(call *ast.CallExpr) bool {
	fn, ok := funcObjectForCall(c.pass, call.Fun)
	if !ok || fn.Pkg() == nil {
		return false
	}
	return fn.Name() == "RunSubprocess" && strings.HasSuffix(fn.Pkg().Path(), dispatchSuffix)
}

func (c checker) applyAssignments(lhs, rhs []ast.Expr, state *liveState) {
	for i, left := range lhs {
		ident, ok := left.(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}
		v, ok := objectForIdent(c.pass, ident).(*types.Var)
		if !ok || !isPlanSessionType(v.Type()) {
			continue
		}
		right := assignmentRHSAt(rhs, i)
		if right == nil || isNilIdent(right) {
			delete(state.varGroups, v)
			continue
		}
		if groups := c.groupsForExpr(right, state); len(groups) > 0 {
			state.assignGroups(v, groups)
			continue
		}
		if isPlanSessionType(c.typeOfAssignment(right, i, len(lhs))) {
			state.assignNew(v, ident.Name)
		}
	}
}

func (c checker) groupsForExpr(expr ast.Expr, state *liveState) groupSet {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	v, ok := objectForIdent(c.pass, ident).(*types.Var)
	if !ok {
		return nil
	}
	return state.varGroups[v]
}

func assignmentRHSAt(rhs []ast.Expr, i int) ast.Expr {
	if len(rhs) == 1 {
		return rhs[0]
	}
	if i < len(rhs) {
		return rhs[i]
	}
	return nil
}

func (c checker) typeOfAssignment(expr ast.Expr, tupleIndex, lhsCount int) types.Type {
	if expr == nil {
		return nil
	}
	typ := c.pass.TypesInfo.TypeOf(expr)
	if lhsCount > 1 {
		if tuple, ok := typ.(*types.Tuple); ok && tupleIndex < tuple.Len() {
			return tuple.At(tupleIndex).Type()
		}
	}
	return typ
}

func objectForIdent(pass *analysis.Pass, ident *ast.Ident) types.Object {
	if ident == nil {
		return nil
	}
	if obj := pass.TypesInfo.Defs[ident]; obj != nil {
		return obj
	}
	return pass.TypesInfo.Uses[ident]
}

func funcObjectForCall(pass *analysis.Pass, expr ast.Expr) (*types.Func, bool) {
	switch expr := expr.(type) {
	case *ast.Ident:
		fn, ok := objectForIdent(pass, expr).(*types.Func)
		return fn, ok
	case *ast.SelectorExpr:
		fn, ok := pass.TypesInfo.Uses[expr.Sel].(*types.Func)
		return fn, ok
	}
	return nil, false
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func isPlanSessionType(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "PlanSession" && strings.HasSuffix(named.Obj().Pkg().Path(), planrepoSuffix)
}

func containsPlanSession(t types.Type, seen map[types.Type]bool) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if seen[t] {
		return false
	}
	seen[t] = true
	if isPlanSessionType(t) {
		return true
	}
	switch t := t.(type) {
	case *types.Pointer:
		return containsPlanSession(t.Elem(), seen)
	case *types.Slice:
		return containsPlanSession(t.Elem(), seen)
	case *types.Array:
		return containsPlanSession(t.Elem(), seen)
	case *types.Map:
		return containsPlanSession(t.Key(), seen) || containsPlanSession(t.Elem(), seen)
	case *types.Chan:
		return containsPlanSession(t.Elem(), seen)
	case *types.Signature:
		return tupleContainsPlanSession(t.Params(), seen) || tupleContainsPlanSession(t.Results(), seen)
	case *types.Named:
		return containsPlanSession(t.Underlying(), seen)
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if containsPlanSession(t.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func tupleContainsPlanSession(tuple *types.Tuple, seen map[types.Type]bool) bool {
	if tuple == nil {
		return false
	}
	for i := 0; i < tuple.Len(); i++ {
		if containsPlanSession(tuple.At(i).Type(), seen) {
			return true
		}
	}
	return false
}

func isBackendType(t types.Type) bool {
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "Backend" && strings.HasSuffix(named.Obj().Pkg().Path(), backendSuffix)
}

func isHTTPClientType(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "Client" && named.Obj().Pkg().Path() == "net/http"
}

func isHTTPRoundTripperType(t types.Type) bool {
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "RoundTripper" && named.Obj().Pkg().Path() == "net/http"
}

func isExecCmdType(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "Cmd" && named.Obj().Pkg().Path() == "os/exec"
}

func formatNode(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, node); err != nil {
		return fmt.Sprintf("%T", node)
	}
	return buf.String()
}
