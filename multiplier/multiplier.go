/*
The multiplier package provides a collection of functions used to help scale tests with
the complexity multipliers present in the production code.

A complexity multiplier represents a concept that multiplies the surface area and complexity
of other proximal features, for example, database transactions are complexity modifiers, as
when adding many new database actions such as a new filter operation, the new action must be
tested both with, and without a transaction - the transaction concept multiplies the complexity
of the system.

Complexity multipliers are represented in this package by the [Multiplier] interface.  Concrete
implementations of this interface should be defined in the packages consuming this package, and then
registered by calling [Register].  This is typically done in an `init` function in the package defining
the concrete type.

Once all desired [Multiplier]s have been registered, [Init] should be called in order to select
which of the registered multipliers are active and should be used to modify the set of actions provided
to [Apply].

[Apply] should be called once per test, and will modify the given set of actions in order to test the
active complexity multipliers.

[Multiplier]s do not have any impact during action execution, they act only on the action set itself,
redefining, creating or deleting elements from the test definition *before* the action set executes.

# Action-Aware Skipping

Multipliers may optionally implement [ActionAwareSkipper] to provide dynamic skip logic based on
the test's action set. When [Skip] is called with a non-nil actions parameter, multipliers
implementing this interface will be consulted to determine if the test should be skipped.
*/
package multiplier

import (
	"os"
	"strings"
	"testing"

	"github.com/sourcenetwork/testo/action"
)

var activeMultipliers []Multiplier
var availableMultipliers []Multiplier

// Name represents the unique name of a multiplier.
//
// The alias provides a more descriptive type to use besides `string`.
type Name = string

// Multiplier represents a complexity multiplier of the system under test.
//
// It represents a concept that multiplies the surface area and complexity
// of other proximal features, for example, database transactions are
// complexity modifiers, as when adding many new database actions such as
// a new filter operation, the new action must be tested both with, and without
// a transaction - the transaction concept multiplies the complexity of the system.
//
// The identification and automatic representation of complexity multipiers are critical
// to the long term maintainability and scalability of a codebase. Without them, introducing
// fairly simple features becomes high risk, and requires a forever growing number of manually
// constructed tests, when the feature developer needs to remember, and write tests for every
// multiplier that affects the feature under test.  This is error prone, very tedious, and
// distracts from the feature itself - often degrading the quality of both test and production code.
//
// Complexity multipliers defined using this type may redefine the set of test actions
// to perform within a test, allowing a single test to scale with the complexity
// multiplier.  For example, applying a 'namespace' multplier may add an action that
// namespaces the store under test, reducing, but not removing, testing cost of the
// namespace multiplier.
type Multiplier interface {
	// Name returns the unique name of the multiplier.
	Name() Name

	// Apply applies the complexity multiplier to the given action set, returning a new
	// action set testing the multiplier.
	Apply(source action.Actions) action.Actions
}

// ActionAwareSkipper is an optional interface that [Multiplier] implementations may
// implement to provide action-based test skipping logic.
//
// When a multiplier implements this interface, [Skip] will call [ShouldSkip]
// to determine if the test should be skipped based on the action set. This allows
// multipliers to skip tests that are incompatible with or irrelevant to the multiplier's
// purpose.
//
// This interface is optional - multipliers that do not implement it will not participate
// in action-based skip decisions.
type ActionAwareSkipper interface {
	// ShouldSkip returns true if the test should be skipped based on the given action set.
	//
	// Implementations should return true when the action set contains elements that make
	// the multiplier inapplicable or when running the multiplier would be redundant.
	ShouldSkip(actions action.Actions) bool
}

// Register adds the given multiplier to the internal set of available multipliers.
//
// Multipliers must be registered before calling [Init] in order to be applied.
func Register(multiplier Multiplier) {
	availableMultipliers = append(availableMultipliers, multiplier)
}

// Init initializes the multiplier engine, determining which multipliers are to
// be applied to action sets.
//
// It must be called after all required [Multiplier]s are [Register]ed.
//
// It will check to see if the given environment variable is set, and if so,
// sources the comma sperated set of multiplier names from it.  If the
// environment variable is not set, it will use the provided defaults.
//
// Multipliers will be applied in the order in which they are given.
func Init(envVarName string, defaults ...Name) {
	var multiplierNames []string
	multipliersString, ok := os.LookupEnv(envVarName)
	if ok {
		multiplierNames = strings.Split(multipliersString, ",")
	} else {
		multiplierNames = defaults
	}

	for i, name := range multiplierNames {
		multiplierNames[i] = strings.TrimSpace(name)
	}

	availableMultipliersByName := make(map[Name]Multiplier, len(availableMultipliers))
	for _, multiplier := range availableMultipliers {
		availableMultipliersByName[multiplier.Name()] = multiplier
	}

	activeMultipliers = make([]Multiplier, 0, len(multiplierNames))
	for _, multiplierName := range multiplierNames {
		if multiplier, ok := availableMultipliersByName[multiplierName]; ok {
			activeMultipliers = append(activeMultipliers, multiplier)
		}
	}
}

// Apply applies all active multipliers to the given action set, in the order in which the
// multipliers are provided.
func Apply(actions action.Actions) action.Actions {
	for _, multiplier := range activeMultipliers {
		actions = multiplier.Apply(actions)
	}

	return actions
}

// Skip skips the test if:
//   - The active set of multipliers does not contain a multiplier with a name exactly matching
//     a value in the given `includes` set.
//   - The active set of multipliers contains a multiplier with a name exactly matching
//     a value in the given `excludes` set.
//   - Any active multiplier implements [ActionAwareSkipper] and returns true from [ShouldSkip]
//     for the given action set.
func Skip(t testing.TB, actions action.Actions, includes []Name, excludes []Name) {
	for _, multiplier := range activeMultipliers {
		for _, exclude := range excludes {
			if multiplier.Name() == exclude {
				t.Skipf("skipping, multiplier is excluded. Name: %s", exclude)
			}
		}
	}

	for _, include := range includes {
		included := false
		for _, multiplier := range activeMultipliers {
			if multiplier.Name() == include {
				included = true
				break
			}
		}

		if !included {
			t.Skipf("skipping, required multiplier is not included. Name: %s", include)
		}
	}

	for _, multiplier := range activeMultipliers {
		if skipper, ok := multiplier.(ActionAwareSkipper); ok {
			if skipper.ShouldSkip(actions) {
				t.Skipf("skipping, multiplier %s requested skip based on actions", multiplier.Name())
			}
		}
	}
}

// Get returns a comma-seperated string containing the current active multiplier names.
func Get() string {
	multipliers := make([]string, len(activeMultipliers))
	for i, multiplier := range activeMultipliers {
		multipliers[i] = multiplier.Name()
	}

	return strings.Join(multipliers, ",")
}
