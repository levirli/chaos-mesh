// Copyright 2021 Chaos Mesh Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package fis

import (
	"net/http"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fis"
	fistypes "github.com/aws/aws-sdk-go-v2/service/fis/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	u "github.com/chaos-mesh/chaos-mesh/pkg/dashboard/apiserver/utils"
)

// targetName is the logical target name inside the experiment template.
const targetName = "chaos-target"

// actionName is the logical action name inside the experiment template.
const actionName = "chaos-action"

// nameTagKey carries a human-readable template/experiment name.
const nameTagKey = "Name"

// templateIDTagKey carries the source template id on a started experiment.
const templateIDTagKey = "chaos-mesh/template-id"

// Register mounts the FIS endpoints on the given router group.
func Register(r *gin.RouterGroup, s *Service) {
	g := r.Group("/aws/fis")
	{
		g.GET("/templates", s.listTemplates)
		g.POST("/templates", s.createTemplate)
		g.GET("/templates/:id", s.getTemplate)
		g.PUT("/templates/:id", s.updateTemplate)
		g.DELETE("/templates/:id", s.deleteTemplate)
		g.POST("/templates/:id/start", s.startExperiment)
		g.GET("/experiments", s.listExperiments)
		g.GET("/experiments/:id", s.getExperiment)
		g.POST("/experiments/:id/stop", s.stopExperiment)
	}
}

// resolvedInputs holds the target selection and parameters derived from a
// CreateTemplateRequest via its actionMeta. It is the single source of truth
// shared by createTemplate and updateTemplate so the two never drift.
type resolvedInputs struct {
	resourceArns []string
	resourceTags map[string]string
	targetParams map[string]string
	actionParams map[string]string
}

// resolveTemplateInputs validates a request against its action metadata and
// builds the target selection + action/target parameters. Returned errors are
// user input errors (map to HTTP 400).
func resolveTemplateInputs(req *CreateTemplateRequest, meta actionMeta) (*resolvedInputs, error) {
	r := &resolvedInputs{}
	switch meta.selection {
	case targetByArn:
		if req.TargetArn == nil || *req.TargetArn == "" {
			return nil, errors.Errorf("targetArn is required for action %s", req.Action)
		}
		r.resourceArns = []string{*req.TargetArn}
	case targetByTags:
		if len(req.ResourceTags) == 0 {
			return nil, errors.Errorf("resourceTags is required for action %s", req.Action)
		}
		r.resourceTags = req.ResourceTags
	}
	if meta.targetParameters != nil {
		r.targetParams = meta.targetParameters(req)
	}
	if meta.actionParameters != nil {
		ap, err := meta.actionParameters(req)
		if err != nil {
			return nil, err
		}
		r.actionParams = ap
	}
	return r, nil
}

// @Summary List AWS FIS experiment templates.
// @Description List experiment templates in the configured AWS region.
// @Tags aws_fis
// @Produce json
// @Success 200 {array} fis.TemplateSummary
// @Failure 500 {object} u.APIError
// @Router /aws/fis/templates [get]
func (s *Service) listTemplates(c *gin.Context) {
	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out := []TemplateSummary{}
	paginator := fis.NewListExperimentTemplatesPaginator(client, &fis.ListExperimentTemplatesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(c.Request.Context())
		if err != nil {
			u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
			return
		}
		for i := range page.ExperimentTemplates {
			t := page.ExperimentTemplates[i]
			out = append(out, summaryFromTemplate(t))
		}
	}

	c.JSON(http.StatusOK, out)
}

// @Summary Create an AWS FIS experiment template.
// @Description Build and submit a CreateExperimentTemplate request from a CreateTemplateRequest body.
// @Tags aws_fis
// @Accept json
// @Produce json
// @Param body body fis.CreateTemplateRequest true "Template parameters"
// @Success 200 {object} fis.createTemplateResponse
// @Failure 400 {object} u.APIError
// @Failure 500 {object} u.APIError
// @Router /aws/fis/templates [post]
func (s *Service) createTemplate(c *gin.Context) {
	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		u.SetAPIError(c, u.ErrBadRequest.WrapWithNoMessage(err))
		return
	}

	meta, ok := actionTable[req.Action]
	if !ok {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.Errorf("unsupported AWS FIS action: %s", req.Action), "invalid action"))
		return
	}

	fisCfg := s.conf.AWSFIS
	if fisCfg.RoleArn == "" {
		u.SetAPIError(c, u.ErrInternalServer.Wrap(errors.New("AWS FIS fixed configuration missing: AWSFIS_ROLE_ARN must be set"), "missing AWS FIS config"))
		return
	}

	resolved, err := resolveTemplateInputs(&req, meta)
	if err != nil {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(err, "invalid template parameters"))
		return
	}
	target := fistypes.CreateExperimentTemplateTargetInput{
		ResourceType:  awssdk.String(meta.resourceType),
		SelectionMode: awssdk.String("ALL"),
		ResourceArns:  resolved.resourceArns,
		ResourceTags:  resolved.resourceTags,
		Parameters:    resolved.targetParams,
	}
	actionParams := resolved.actionParams

	description := req.Description
	if description == "" {
		description = "chaos-mesh template"
	}

	nameTag := req.Name
	if nameTag == "" {
		nameTag = "chaos-mesh/" + uuid.NewString()[:8]
	}

	createInput := &fis.CreateExperimentTemplateInput{
		ClientToken: awssdk.String(uuid.NewString()),
		Description: awssdk.String(description),
		RoleArn:     awssdk.String(fisCfg.RoleArn),
		StopConditions: []fistypes.CreateExperimentTemplateStopConditionInput{
			{Source: awssdk.String("none")},
		},
		ExperimentOptions: &fistypes.CreateExperimentTemplateExperimentOptionsInput{
			AccountTargeting:          fistypes.AccountTargetingSingleAccount,
			EmptyTargetResolutionMode: fistypes.EmptyTargetResolutionModeFail,
		},
		Actions: map[string]fistypes.CreateExperimentTemplateActionInput{
			actionName: {
				ActionId:   awssdk.String(string(req.Action)),
				Parameters: actionParams,
				Targets:    map[string]string{meta.targetKey: targetName},
			},
		},
		Targets: map[string]fistypes.CreateExperimentTemplateTargetInput{
			targetName: target,
		},
		Tags: map[string]string{
			nameTagKey: nameTag,
		},
	}
	if fisCfg.ReportS3Bucket != "" {
		createInput.ExperimentReportConfiguration = &fistypes.CreateExperimentTemplateReportConfigurationInput{
			Outputs: &fistypes.ExperimentTemplateReportConfigurationOutputsInput{
				S3Configuration: &fistypes.ReportConfigurationS3OutputInput{
					BucketName: awssdk.String(fisCfg.ReportS3Bucket),
				},
			},
		}
	}

	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out, err := client.CreateExperimentTemplate(c.Request.Context(), createInput)
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	c.JSON(http.StatusOK, createTemplateResponse{ID: awssdk.ToString(out.ExperimentTemplate.Id)})
}

// @Summary Delete an AWS FIS experiment template.
// @Description Delete the experiment template with the given id.
// @Tags aws_fis
// @Produce json
// @Param id path string true "Template id"
// @Success 204
// @Failure 404 {object} u.APIError
// @Failure 500 {object} u.APIError
// @Router /aws/fis/templates/{id} [delete]
func (s *Service) deleteTemplate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.New("id is required"), "missing id"))
		return
	}

	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	if _, err := client.DeleteExperimentTemplate(c.Request.Context(), &fis.DeleteExperimentTemplateInput{
		Id: awssdk.String(id),
	}); err != nil {
		var notFound *fistypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			u.SetAPIError(c, u.ErrNotFound.WrapWithNoMessage(err))
			return
		}
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Start an AWS FIS experiment from a template.
// @Description Start an experiment from the template with the given id.
// @Tags aws_fis
// @Produce json
// @Param id path string true "Template id"
// @Success 200 {object} fis.startExperimentResponse
// @Failure 404 {object} u.APIError
// @Failure 500 {object} u.APIError
// @Router /aws/fis/templates/{id}/start [post]
func (s *Service) startExperiment(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.New("id is required"), "missing id"))
		return
	}

	clientToken := uuid.NewString()
	tags := map[string]string{
		nameTagKey:      "chaos-mesh/" + id + "-" + clientToken[:8],
		templateIDTagKey: id,
	}

	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out, err := client.StartExperiment(c.Request.Context(), &fis.StartExperimentInput{
		ClientToken:          awssdk.String(clientToken),
		ExperimentTemplateId: awssdk.String(id),
		Tags:                 tags,
	})
	if err != nil {
		var notFound *fistypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			u.SetAPIError(c, u.ErrNotFound.WrapWithNoMessage(err))
			return
		}
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	c.JSON(http.StatusOK, startExperimentResponse{ID: awssdk.ToString(out.Experiment.Id)})
}

// @Summary List AWS FIS experiments.
// @Description List experiments in the configured AWS region.
// @Tags aws_fis
// @Produce json
// @Success 200 {array} fis.ExperimentSummary
// @Failure 500 {object} u.APIError
// @Router /aws/fis/experiments [get]
func (s *Service) listExperiments(c *gin.Context) {
	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out := []ExperimentSummary{}
	paginator := fis.NewListExperimentsPaginator(client, &fis.ListExperimentsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(c.Request.Context())
		if err != nil {
			u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
			return
		}
		for i := range page.Experiments {
			e := page.Experiments[i]
			out = append(out, summaryFromExperiment(e))
		}
	}

	c.JSON(http.StatusOK, out)
}

// @Summary Get an AWS FIS experiment template.
// @Description Get the full experiment template (including actions/targets) by id.
// @Tags aws_fis
// @Produce json
// @Param id path string true "Template id"
// @Success 200 {object} fis.TemplateDetail
// @Failure 404 {object} u.APIError
// @Failure 500 {object} u.APIError
// @Router /aws/fis/templates/{id} [get]
func (s *Service) getTemplate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.New("id is required"), "missing id"))
		return
	}

	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out, err := client.GetExperimentTemplate(c.Request.Context(), &fis.GetExperimentTemplateInput{
		Id: awssdk.String(id),
	})
	if err != nil {
		var notFound *fistypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			u.SetAPIError(c, u.ErrNotFound.WrapWithNoMessage(err))
			return
		}
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	c.JSON(http.StatusOK, detailFromTemplate(out.ExperimentTemplate))
}

// @Summary Update an AWS FIS experiment template.
// @Description Update the description and target/parameters of a template. The
// @Description action id is fixed; only actions supported by the dashboard can be updated.
// @Tags aws_fis
// @Accept json
// @Produce json
// @Param id path string true "Template id"
// @Param body body fis.CreateTemplateRequest true "Updated template parameters"
// @Success 200 {object} fis.TemplateDetail
// @Failure 400 {object} u.APIError
// @Failure 404 {object} u.APIError
// @Failure 500 {object} u.APIError
// @Router /aws/fis/templates/{id} [put]
func (s *Service) updateTemplate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.New("id is required"), "missing id"))
		return
	}

	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		u.SetAPIError(c, u.ErrBadRequest.WrapWithNoMessage(err))
		return
	}

	meta, ok := actionTable[req.Action]
	if !ok {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.Errorf("unsupported AWS FIS action: %s", req.Action), "invalid action"))
		return
	}

	resolved, err := resolveTemplateInputs(&req, meta)
	if err != nil {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(err, "invalid template parameters"))
		return
	}

	description := req.Description
	if description == "" {
		description = "chaos-mesh template"
	}

	updateInput := &fis.UpdateExperimentTemplateInput{
		Id:          awssdk.String(id),
		Description: awssdk.String(description),
		Actions: map[string]fistypes.UpdateExperimentTemplateActionInputItem{
			actionName: {
				ActionId:   awssdk.String(string(req.Action)),
				Parameters: resolved.actionParams,
				Targets:    map[string]string{meta.targetKey: targetName},
			},
		},
		Targets: map[string]fistypes.UpdateExperimentTemplateTargetInput{
			targetName: {
				ResourceType:  awssdk.String(meta.resourceType),
				SelectionMode: awssdk.String("ALL"),
				ResourceArns:  resolved.resourceArns,
				ResourceTags:  resolved.resourceTags,
				Parameters:    resolved.targetParams,
			},
		},
	}

	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out, err := client.UpdateExperimentTemplate(c.Request.Context(), updateInput)
	if err != nil {
		var notFound *fistypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			u.SetAPIError(c, u.ErrNotFound.WrapWithNoMessage(err))
			return
		}
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	c.JSON(http.StatusOK, detailFromTemplate(out.ExperimentTemplate))
}

// @Summary Get an AWS FIS experiment.
// @Description Get a single experiment (including state and actions/targets) by id.
// @Tags aws_fis
// @Produce json
// @Param id path string true "Experiment id"
// @Success 200 {object} fis.ExperimentDetail
// @Failure 404 {object} u.APIError
// @Failure 500 {object} u.APIError
// @Router /aws/fis/experiments/{id} [get]
func (s *Service) getExperiment(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.New("id is required"), "missing id"))
		return
	}

	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out, err := client.GetExperiment(c.Request.Context(), &fis.GetExperimentInput{
		Id: awssdk.String(id),
	})
	if err != nil {
		var notFound *fistypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			u.SetAPIError(c, u.ErrNotFound.WrapWithNoMessage(err))
			return
		}
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	c.JSON(http.StatusOK, detailFromExperiment(out.Experiment))
}

// @Summary Stop an AWS FIS experiment.
// @Description Stop a running experiment by id. Only meaningful for stoppable
// @Description (non one-shot) faults; the UI shows the button only for running experiments.
// @Tags aws_fis
// @Produce json
// @Param id path string true "Experiment id"
// @Success 200 {object} fis.stopExperimentResponse
// @Failure 404 {object} u.APIError
// @Failure 500 {object} u.APIError
// @Router /aws/fis/experiments/{id}/stop [post]
func (s *Service) stopExperiment(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		u.SetAPIError(c, u.ErrBadRequest.Wrap(errors.New("id is required"), "missing id"))
		return
	}

	client, err := s.newFISClient(c.Request.Context())
	if err != nil {
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	out, err := client.StopExperiment(c.Request.Context(), &fis.StopExperimentInput{
		Id: awssdk.String(id),
	})
	if err != nil {
		var notFound *fistypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			u.SetAPIError(c, u.ErrNotFound.WrapWithNoMessage(err))
			return
		}
		u.SetAPIError(c, u.ErrInternalServer.WrapWithNoMessage(err))
		return
	}

	c.JSON(http.StatusOK, stopExperimentResponse{ID: awssdk.ToString(out.Experiment.Id)})
}

// firstTemplateAction returns the single action of a template (templates built
// by this dashboard have exactly one). Returns the zero value if none.
func firstTemplateAction(t fistypes.ExperimentTemplate) fistypes.ExperimentTemplateAction {
	for _, a := range t.Actions {
		return a
	}
	return fistypes.ExperimentTemplateAction{}
}

// firstTemplateTarget returns the single target of a template. Returns the zero
// value if none.
func firstTemplateTarget(t fistypes.ExperimentTemplate) fistypes.ExperimentTemplateTarget {
	for _, tg := range t.Targets {
		return tg
	}
	return fistypes.ExperimentTemplateTarget{}
}

// detailFromTemplate projects a full FIS ExperimentTemplate into TemplateDetail.
func detailFromTemplate(t *fistypes.ExperimentTemplate) TemplateDetail {
	d := TemplateDetail{}
	if t == nil {
		return d
	}
	d.ID = awssdk.ToString(t.Id)
	d.Description = awssdk.ToString(t.Description)
	d.RoleArn = awssdk.ToString(t.RoleArn)
	d.Tags = t.Tags
	if t.CreationTime != nil {
		d.CreationTime = *t.CreationTime
	}
	if t.LastUpdateTime != nil {
		d.LastUpdateTime = *t.LastUpdateTime
	}

	action := firstTemplateAction(*t)
	d.Action = awssdk.ToString(action.ActionId)
	d.ActionParameters = action.Parameters

	target := firstTemplateTarget(*t)
	d.ResourceType = awssdk.ToString(target.ResourceType)
	d.SelectionMode = awssdk.ToString(target.SelectionMode)
	if len(target.ResourceArns) > 0 {
		d.TargetArn = target.ResourceArns[0]
	}
	d.ResourceTags = target.ResourceTags
	d.TargetParameters = target.Parameters

	return d
}

// detailFromExperiment projects a full FIS Experiment into ExperimentDetail.
func detailFromExperiment(e *fistypes.Experiment) ExperimentDetail {
	d := ExperimentDetail{}
	if e == nil {
		return d
	}
	d.ID = awssdk.ToString(e.Id)
	d.TemplateID = awssdk.ToString(e.ExperimentTemplateId)
	d.Tags = e.Tags
	if e.State != nil {
		d.State = string(e.State.Status)
		d.StateReason = awssdk.ToString(e.State.Reason)
		if e.State.Error != nil {
			d.Error = awssdk.ToString(e.State.Error.Code)
		}
	}
	if e.CreationTime != nil {
		d.CreationTime = *e.CreationTime
	}
	d.StartTime = e.StartTime
	d.EndTime = e.EndTime

	for _, a := range e.Actions {
		d.Action = awssdk.ToString(a.ActionId)
		d.ActionParameters = a.Parameters
		break
	}
	for _, tg := range e.Targets {
		d.ResourceType = awssdk.ToString(tg.ResourceType)
		d.TargetArns = tg.ResourceArns
		d.ResourceTags = tg.ResourceTags
		break
	}

	return d
}

// summaryFromTemplate projects a FIS ExperimentTemplateSummary into the
// dashboard response shape. Action is left empty because the AWS
// ListExperimentTemplates API does not return the actions map; the UI can
// fetch details on demand if needed.
func summaryFromTemplate(t fistypes.ExperimentTemplateSummary) TemplateSummary {
	s := TemplateSummary{
		Description: awssdk.ToString(t.Description),
		Tags:        t.Tags,
	}
	if t.Id != nil {
		s.ID = *t.Id
	}
	if t.CreationTime != nil {
		s.CreationTime = *t.CreationTime
	}
	return s
}

// summaryFromExperiment projects a FIS ExperimentSummary into the dashboard
// response shape. TemplateID is extracted from the chaos-mesh/template-id tag
// when present.
func summaryFromExperiment(e fistypes.ExperimentSummary) ExperimentSummary {
	s := ExperimentSummary{
		TemplateID: awssdk.ToString(e.ExperimentTemplateId),
		Tags:       e.Tags,
	}
	if e.Id != nil {
		s.ID = *e.Id
	}
	if e.CreationTime != nil {
		s.CreationTime = *e.CreationTime
	}
	if e.State != nil {
		s.State = string(e.State.Status)
	}
	if e.Tags != nil {
		if v, ok := e.Tags[templateIDTagKey]; ok {
			s.TemplateID = v
		}
	}
	return s
}
