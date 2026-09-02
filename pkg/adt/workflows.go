package adt

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// --- Workflow Tools ---
// These tools combine multiple operations into atomic workflows for simpler usage.

// WriteProgramResult represents the result of writing a program.
type WriteProgramResult struct {
	Success      bool                `json:"success"`
	ProgramName  string              `json:"programName"`
	ObjectURL    string              `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	Message      string              `json:"message,omitempty"`
}

// WriteProgram performs SyntaxCheck -> Lock -> UpdateSource -> Unlock -> Activate.
// The check comes first on purpose: it is a stateless request, and sent while
// a lock is held it ends the stateful session the lock belongs to (issue #88).
// This is a convenience method for updating existing programs.
func (c *Client) WriteProgram(ctx context.Context, programName string, source string, transport string) (*WriteProgramResult, error) {
	programName = strings.ToUpper(programName)
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(programName))
	sourceURL := objectURL + "/source/main"

	// Unified mutation policy gate (op type + package + transport). The
	// returned context carries the mark that stops UpdateSource resolving the
	// same package again from inside the lock window (issue #91).
	ctx, err := c.gateAndMark(ctx, MutationContext{
		Op:        OpWorkflow,
		OpName:    "WriteProgram",
		ObjectURL: objectURL,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}

	result := &WriteProgramResult{
		ProgramName: programName,
		ObjectURL:   objectURL,
	}

	// Step 1: Syntax check before making changes
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}

	// Check for syntax errors
	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = "Source has syntax errors - not saved"
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors // Include warnings if any

	// Step 2: Lock the object
	lock, err := c.LockObject(ctx, objectURL, "MODIFY", transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Ensure we unlock on any error. Detached from the caller's cancellation,
	// and reported rather than dropped: see the same defer in WriteInclude.
	unlocked := false
	defer func() {
		if !unlocked && !result.Success {
			if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
				result.Message = joinMessage(result.Message, strandedLockAdvice(objectURL, unlockErr))
			}
		}
	}()

	// Reuse the request the object is already bound to when the caller supplied no
	// transport, so an already-captured object is not rejected with a spurious 409
	// (issue #144). Re-checks transportable-edit policy on the resolved request.
	transport, err = c.resolveWriteTransport(transport, lock.CorrNr, "WriteProgram")
	if err != nil {
		result.Message = fmt.Sprintf("Transportable-edit check failed: %v", err)
		return result, nil
	}

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 4: Unlock before activation (SAP requirement)
	// Detached, and marked released before the attempt, for the reasons
	// writeFunctionModule's happy path is: an unlock issued on a context that
	// has just expired never leaves the process, and the defer must not send a
	// second UNLOCK for a handle this one already consumed.
	unlocked = true
	if err = c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); err != nil {
		result.Message = strandedLockAdvice(objectURL, err)
		return result, nil
	}

	// Step 5: Activate
	activation, err := c.Activate(ctx, objectURL, programName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Program updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}

// WriteIncludeResult represents the result of writing an ABAP include.
type WriteIncludeResult struct {
	Success      bool                `json:"success"`
	IncludeName  string              `json:"includeName"`
	ObjectURL    string              `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	Message      string              `json:"message,omitempty"`
}

// WriteInclude performs SyntaxCheck -> Lock -> UpdateSource -> Unlock -> Activate
// for an ABAP include. Check before lock: see WriteProgram.
func (c *Client) WriteInclude(ctx context.Context, includeName string, source string, transport string) (*WriteIncludeResult, error) {
	includeName = strings.ToUpper(includeName)
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/includes/%s", url.PathEscape(includeName))
	sourceURL := objectURL + "/source/main"

	// Unified mutation policy gate (op type + package + transport). The
	// returned context carries the mark that stops UpdateSource resolving the
	// same package again from inside the lock window (issue #91) — the include
	// path needs it exactly as much as WriteProgram and WriteClass do.
	ctx, err := c.gateAndMark(ctx, MutationContext{
		Op:        OpWorkflow,
		OpName:    "WriteInclude",
		ObjectURL: objectURL,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}

	result := &WriteIncludeResult{
		IncludeName: includeName,
		ObjectURL:   objectURL,
	}

	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}
	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = "Source has syntax errors - not saved"
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors

	lock, err := c.LockObject(ctx, objectURL, "MODIFY", transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}
	unlocked := false
	defer func() {
		if !unlocked && !result.Success {
			// Detached from the caller's cancellation, and reported rather
			// than dropped: when the write failed *because* the context was
			// cancelled or hit its deadline, an unlock on that same context
			// dies inside http.NewRequestWithContext without sending a byte
			// and the ENQUEUE stays on the include. The caller's only evidence
			// of that is this message.
			if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
				result.Message = joinMessage(result.Message, strandedLockAdvice(objectURL, unlockErr))
			}
		}
	}()

	// Reuse the request the object is already bound to when the caller supplied no
	// transport, so an already-captured object is not rejected with a spurious 409
	// (issue #144). Re-checks transportable-edit policy on the resolved request.
	transport, err = c.resolveWriteTransport(transport, lock.CorrNr, "WriteInclude")
	if err != nil {
		result.Message = fmt.Sprintf("Transportable-edit check failed: %v", err)
		return result, nil
	}

	if err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, transport); err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Detached, and marked released before the attempt, for the reasons
	// writeFunctionModule's happy path is: an unlock issued on a context that
	// has just expired never leaves the process, and the defer must not send a
	// second UNLOCK for a handle this one already consumed.
	unlocked = true
	if err = c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); err != nil {
		result.Message = strandedLockAdvice(objectURL, err)
		return result, nil
	}

	activation, err := c.Activate(ctx, objectURL, includeName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Include updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}
	return result, nil
}

// WriteClassResult represents the result of writing a class.
type WriteClassResult struct {
	Success      bool                `json:"success"`
	ClassName    string              `json:"className"`
	ObjectURL    string              `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	Message      string              `json:"message,omitempty"`
}

// WriteClass performs SyntaxCheck -> Lock -> UpdateSource -> Unlock -> Activate
// for classes. Check before lock: see WriteProgram.
func (c *Client) WriteClass(ctx context.Context, className string, source string, transport string) (*WriteClassResult, error) {
	className = strings.ToUpper(className)
	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(className))
	sourceURL := objectURL + "/source/main"

	// Unified mutation policy gate (op type + package + transport). The
	// returned context carries the mark that stops UpdateSource resolving the
	// same package again from inside the lock window (issue #91).
	ctx, err := c.gateAndMark(ctx, MutationContext{
		Op:        OpWorkflow,
		OpName:    "WriteClass",
		ObjectURL: objectURL,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}

	result := &WriteClassResult{
		ClassName: className,
		ObjectURL: objectURL,
	}

	// Step 1: Syntax check
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}

	// Check for syntax errors
	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = "Source has syntax errors - not saved"
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors

	// Step 2: Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY", transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Detached from the caller's cancellation, and reported rather than
	// dropped: see the same defer in WriteInclude.
	unlocked := false
	defer func() {
		if !unlocked && !result.Success {
			if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
				result.Message = joinMessage(result.Message, strandedLockAdvice(objectURL, unlockErr))
			}
		}
	}()

	// Reuse the request the object is already bound to when the caller supplied no
	// transport, so an already-captured object is not rejected with a spurious 409
	// (issue #144). Re-checks transportable-edit policy on the resolved request.
	transport, err = c.resolveWriteTransport(transport, lock.CorrNr, "WriteClass")
	if err != nil {
		result.Message = fmt.Sprintf("Transportable-edit check failed: %v", err)
		return result, nil
	}

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 4: Unlock
	// Detached, and marked released before the attempt, for the reasons
	// writeFunctionModule's happy path is: an unlock issued on a context that
	// has just expired never leaves the process, and the defer must not send a
	// second UNLOCK for a handle this one already consumed.
	unlocked = true
	if err = c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); err != nil {
		result.Message = strandedLockAdvice(objectURL, err)
		return result, nil
	}

	// Step 5: Activate
	activation, err := c.Activate(ctx, objectURL, className)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Class updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}

// CreateProgramResult represents the result of creating a program.
type CreateProgramResult struct {
	Success      bool                `json:"success"`
	ProgramName  string              `json:"programName"`
	ObjectURL    string              `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	Message      string              `json:"message,omitempty"`
}

// CreateAndActivateProgram creates a new program with source code and activates it.
// Workflow: CreateObject -> Lock -> UpdateSource -> Unlock -> Activate
func (c *Client) CreateAndActivateProgram(ctx context.Context, programName string, description string, packageName string, source string, transport string) (*CreateProgramResult, error) {
	programName = strings.ToUpper(programName)
	packageName = strings.ToUpper(packageName)

	// Unified mutation policy gate (op type + package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpWorkflow,
		OpName:    "CreateAndActivateProgram",
		Package:   packageName,
		Transport: transport,
	}); err != nil {
		return nil, err
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(programName))
	sourceURL := objectURL + "/source/main"

	result := &CreateProgramResult{
		ProgramName: programName,
		ObjectURL:   objectURL,
	}

	// Step 1: Create the program
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: description,
		PackageName: packageName,
		Transport:   transport,
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create program: %v", err)
		return result, nil
	}

	// The gate above accepted packageName, and CreateObject gated it a second
	// time before asking SAP to put the program there — so the program's
	// package is a package the whitelist allows. Record that for the object,
	// or UpdateSource resolves it again from inside the lock (issue #91).
	ctx = withMutationPackageChecked(ctx, objectURL)

	// Step 2: Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY", transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Detached from the caller's cancellation, and reported rather than
	// dropped: see the same defer in WriteInclude.
	unlocked := false
	defer func() {
		if !unlocked && !result.Success {
			if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
				result.Message = joinMessage(result.Message, strandedLockAdvice(objectURL, unlockErr))
			}
		}
	}()

	// Step 3: Update source
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 4: Unlock
	// Detached, and marked released before the attempt, for the reasons
	// writeFunctionModule's happy path is: an unlock issued on a context that
	// has just expired never leaves the process, and the defer must not send a
	// second UNLOCK for a handle this one already consumed.
	unlocked = true
	if err = c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); err != nil {
		result.Message = strandedLockAdvice(objectURL, err)
		return result, nil
	}

	// Step 5: Activate
	activation, err := c.Activate(ctx, objectURL, programName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Program created and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}

// CreateClassWithTestsResult represents the result of creating a class with unit tests.
type CreateClassWithTestsResult struct {
	Success        bool              `json:"success"`
	ClassName      string            `json:"className"`
	ObjectURL      string            `json:"objectUrl"`
	Activation     *ActivationResult `json:"activation,omitempty"`
	UnitTestResult *UnitTestResult   `json:"unitTestResult,omitempty"`
	Message        string            `json:"message,omitempty"`
}

// CreateClassWithTests creates a new class with unit tests and runs them.
// Workflow: CreateObject -> Lock -> UpdateSource -> CreateTestInclude -> UpdateClassInclude -> Unlock -> Activate -> RunUnitTests
func (c *Client) CreateClassWithTests(ctx context.Context, className string, description string, packageName string, classSource string, testSource string, transport string) (*CreateClassWithTestsResult, error) {
	className = strings.ToUpper(className)
	packageName = strings.ToUpper(packageName)

	// Unified mutation policy gate (op type + package + transport)
	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpWorkflow,
		OpName:    "CreateClassWithTests",
		Package:   packageName,
		Transport: transport,
	}); err != nil {
		return nil, err
	}

	objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(className))
	sourceURL := objectURL + "/source/main"

	result := &CreateClassWithTestsResult{
		ClassName: className,
		ObjectURL: objectURL,
	}

	// Step 1: Create the class
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeClass,
		Name:        className,
		Description: description,
		PackageName: packageName,
		Transport:   transport,
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create class: %v", err)
		return result, nil
	}

	// Same reasoning as CreateAndActivateProgram: packageName passed the gate
	// twice and the class was created there, so the three mutators that run
	// under the single lock below (UpdateSource, CreateTestInclude,
	// UpdateClassInclude — all of which resolve to this class URL) need not
	// each resolve the package again mid-window (issue #91).
	ctx = withMutationPackageChecked(ctx, objectURL)

	// Step 2: Lock
	lock, err := c.LockObject(ctx, objectURL, "MODIFY", transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock object: %v", err)
		return result, nil
	}

	// Detached from the caller's cancellation, and reported rather than
	// dropped: see the same defer in WriteInclude.
	unlocked := false
	defer func() {
		if !unlocked && !result.Success {
			if unlockErr := c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); unlockErr != nil {
				result.Message = joinMessage(result.Message, strandedLockAdvice(objectURL, unlockErr))
			}
		}
	}()

	// Step 3: Update main source
	err = c.UpdateSource(ctx, sourceURL, classSource, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update class source: %v", err)
		return result, nil
	}

	// Step 4: Create test include
	err = c.CreateTestInclude(ctx, className, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create test include: %v", err)
		return result, nil
	}

	// Step 5: Update test include
	err = c.UpdateClassInclude(ctx, className, ClassIncludeTestClasses, testSource, lock.LockHandle, transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update test source: %v", err)
		return result, nil
	}

	// Step 6: Unlock
	// Detached, and marked released before the attempt, for the reasons
	// writeFunctionModule's happy path is: an unlock issued on a context that
	// has just expired never leaves the process, and the defer must not send a
	// second UNLOCK for a handle this one already consumed.
	unlocked = true
	if err = c.releaseLockAfterFailure(ctx, objectURL, lock.LockHandle); err != nil {
		result.Message = strandedLockAdvice(objectURL, err)
		return result, nil
	}

	// Step 7: Activate
	activation, err := c.Activate(ctx, objectURL, className)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}
	result.Activation = activation

	if !activation.Success {
		result.Message = "Activation failed - check activation messages"
		return result, nil
	}

	// Step 8: Run unit tests
	flags := DefaultUnitTestFlags()
	testResult, err := c.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		result.Message = fmt.Sprintf("Class activated but unit tests failed to run: %v", err)
		result.Success = true // Class was created successfully
		return result, nil
	}

	result.UnitTestResult = testResult
	result.Success = true
	result.Message = "Class created, activated, and unit tests executed successfully"

	return result, nil
}
