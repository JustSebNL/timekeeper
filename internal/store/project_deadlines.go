// Copyright (c) 2026 https://github.com/JustSebNL/timekeeper. All rights reserved.

package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

// ProjectDeadlineWatch returns due-soon and overdue Project/milestone items for the dashboard.
func (s *Store) ProjectDeadlineWatch(ctx context.Context) (model.ProjectDeadlineWatch, error) {
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return model.ProjectDeadlineWatch{}, err
	}
	watch := model.ProjectDeadlineWatch{Items: make([]model.ProjectDeadlineItem, 0)}
	for _, project := range projects {
		if project.DeadlineState == "due_soon" || project.DeadlineState == "overdue" {
			watch.Items = append(watch.Items, model.ProjectDeadlineItem{
				ProjectID: project.ProjectID, ProjectName: project.ProjectName, EntityType: "project",
				EntityID: project.ProjectID, Name: project.ProjectName, TargetDate: project.TargetEndDate,
				Status: project.Status, DeadlineState: project.DeadlineState, DaysToDeadline: project.DaysToDeadline,
			})
		}
		milestones, err := s.ListProjectMilestones(ctx, project.ProjectID)
		if err != nil {
			return model.ProjectDeadlineWatch{}, fmt.Errorf("load milestones for Project %s: %w", project.ProjectID, err)
		}
		for _, milestone := range milestones {
			if milestone.DeadlineState != "due_soon" && milestone.DeadlineState != "overdue" {
				continue
			}
			watch.Items = append(watch.Items, model.ProjectDeadlineItem{
				ProjectID: project.ProjectID, ProjectName: project.ProjectName, EntityType: "milestone",
				EntityID: milestone.MilestoneID, Name: milestone.Name, TargetDate: milestone.TargetDate,
				Status: string(milestone.Status), DeadlineState: milestone.DeadlineState, DaysToDeadline: milestone.DaysToDeadline,
			})
		}
	}
	sort.Slice(watch.Items, func(i, j int) bool {
		left, right := watch.Items[i], watch.Items[j]
		if left.DeadlineState != right.DeadlineState {
			return left.DeadlineState == "overdue"
		}
		if left.TargetDate != right.TargetDate {
			return left.TargetDate < right.TargetDate
		}
		if !strings.EqualFold(left.ProjectName, right.ProjectName) {
			return strings.ToLower(left.ProjectName) < strings.ToLower(right.ProjectName)
		}
		if left.EntityType != right.EntityType {
			return left.EntityType == "project"
		}
		return left.EntityID < right.EntityID
	})
	return watch, nil
}
