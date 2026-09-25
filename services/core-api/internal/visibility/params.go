package visibility

import "fmt"

// Params allocates positional query parameters for the visibility predicates. It
// exists so a predicate can be embedded in an existing query without renumbering the
// caller's placeholders by hand.
type Params struct {
	args []any
}

// NewParams returns an empty parameter list.
func NewParams() *Params {
	return &Params{}
}

// Add appends a value and returns the placeholder that refers to it.
func (p *Params) Add(value any) string {
	p.args = append(p.args, value)
	return fmt.Sprintf("$%d", len(p.args))
}

// Args returns the accumulated arguments for a query.
func (p *Params) Args() []any {
	return p.args
}

// Len returns the number of allocated parameters.
func (p *Params) Len() int {
	return len(p.args)
}

// Exists returns a SQL expression that is true when table holds the row identified
// by id. It is used to tell a deleted record apart from a denied one.
func (p *Params) Exists(table, id string) string {
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM %s WHERE id = %s)`, table, id)
}

// castUUID renders a placeholder as a uuid so it can be compared with uuid columns
// when the argument is the nil uuid of an anonymous actor.
func castUUID(reference string) string {
	return reference + "::uuid"
}
