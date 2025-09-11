package flow

import (
	"maps"

	"github.com/hupe1980/agentmesh/core"
)

// Helpers for deterministically merging multiple tool/function response events
// back into a single composite event following the original call order.
// Last-write-wins for state/artifact deltas; first-wins for transfer/escalate.
// indexByCallID indexes function responses and actions by FunctionCall ID.
func indexByCallID(events []*core.Event) (map[string][]*core.FunctionResponse, map[string]core.EventActions) {
	respByID := make(map[string][]*core.FunctionResponse)
	actionsByID := make(map[string]core.EventActions)

	for _, ev := range events {
		if ev == nil {
			continue
		}
		frs := ev.GetFunctionResponses()
		for _, fr := range frs {
			if fr == nil || fr.ID == "" {
				continue
			}
			respByID[fr.ID] = append(respByID[fr.ID], fr)
			if _, exists := actionsByID[fr.ID]; !exists {
				actionsByID[fr.ID] = ev.Actions
			}
		}
	}

	return respByID, actionsByID
}

// chooseTemplateResponse picks the first response by original call order (fnCalls)
// to serve as the template for merged FunctionResponseEvent.
func chooseTemplateResponse(
	fnCalls []*core.FunctionCall,
	respByID map[string][]*core.FunctionResponse,
) *core.FunctionResponse {
	for _, call := range fnCalls {
		if call == nil || call.ID == "" {
			continue
		}
		if rs := respByID[call.ID]; len(rs) > 0 {
			return rs[0]
		}
	}
	return nil
}

// buildPartsInOrder flattens responses into Parts following original call order.
func buildPartsInOrder(
	fnCalls []*core.FunctionCall,
	respByID map[string][]*core.FunctionResponse,
) []core.Part {
	parts := make([]core.Part, 0)
	for _, call := range fnCalls {
		if call == nil || call.ID == "" {
			continue
		}
		if rs := respByID[call.ID]; len(rs) > 0 {
			for _, fr := range rs {
				parts = append(parts, &core.FunctionResponsePart{FunctionResponse: fr})
			}
		}
	}
	return parts
}

// mergeActionsInOrder merges EventActions in the order of fnCalls.
// - StateDelta/ArtifactDelta: last write wins per key
// - TransferToAgent/Escalate: first set wins by order
// - SkipSummarization: OR-reduce
func mergeActionsInOrder(
	fnCalls []*core.FunctionCall,
	actionsByID map[string]core.EventActions,
) (map[string]any, map[string]int, core.Opt[string], core.Opt[bool], core.Opt[bool]) {
	stateDelta := make(map[string]any)
	artifactDelta := make(map[string]int)
	var transferTo core.Opt[string]
	var escalate core.Opt[bool]
	var skip core.Opt[bool]

	for _, call := range fnCalls {
		if call == nil || call.ID == "" {
			continue
		}
		act, ok := actionsByID[call.ID]
		if !ok {
			continue
		}

		if sd := act.StateDelta.Or(nil); sd != nil {
			maps.Copy(stateDelta, sd)
		}
		if ad := act.ArtifactDelta.Or(nil); ad != nil {
			maps.Copy(artifactDelta, ad)
		}
		if !transferTo.IsSet() && act.TransferToAgent.IsSet() {
			transferTo = act.TransferToAgent
		}
		if !escalate.IsSet() && act.Escalate.IsSet() {
			escalate = act.Escalate
		}
		if act.SkipSummarization.Or(false) {
			skip = core.Some(true)
		}
	}

	return stateDelta, artifactDelta, transferTo, escalate, skip
}

// assembleMergedFunctionResponseEvent constructs a merged FunctionResponse event with
// parts and merged actions applied.
func assembleMergedFunctionResponseEvent(
	runID, agentName string,
	tmplResp *core.FunctionResponse,
	parts []core.Part,
	stateDelta map[string]any,
	artifactDelta map[string]int,
	transferTo core.Opt[string],
	escalate core.Opt[bool],
	skip core.Opt[bool],
) *core.Event {
	merged := core.NewFunctionResponseEvent(
		runID,
		agentName,
		tmplResp.ID,
		tmplResp.Name,
		tmplResp.Response,
	)
	merged.Parts = parts
	if len(stateDelta) > 0 {
		merged.Actions.StateDelta = core.Map(stateDelta)
	}
	if len(artifactDelta) > 0 {
		merged.Actions.ArtifactDelta = core.Map(artifactDelta)
	}
	if transferTo.IsSet() {
		merged.Actions.TransferToAgent = transferTo
	}
	if escalate.IsSet() {
		merged.Actions.Escalate = escalate
	}
	if skip.IsSet() {
		merged.Actions.SkipSummarization = skip
	}

	return merged
}
