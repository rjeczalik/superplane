package canvases

import (
	"context"
	"errors"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/superplanehq/superplane/pkg/database"
	"github.com/superplanehq/superplane/pkg/grpc/actions/messages"
	"github.com/superplanehq/superplane/pkg/models"
	pb "github.com/superplanehq/superplane/pkg/protos/canvases"
	"github.com/superplanehq/superplane/pkg/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func DeleteCanvas(ctx context.Context, registry *registry.Registry, organizationID uuid.UUID, id string) (*pb.DeleteCanvasResponse, error) {
	canvasID, err := uuid.Parse(id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid canvas id: %v", err)
	}

	canvas, err := models.FindCanvas(organizationID, canvasID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if _, templateErr := models.FindCanvasTemplate(canvasID); templateErr == nil {
				return nil, status.Error(codes.FailedPrecondition, "templates are read-only")
			}
		}
		return nil, status.Errorf(codes.NotFound, "canvas not found: %v", err)
	}

	if canvas.IsTemplate {
		return nil, status.Error(codes.FailedPrecondition, "templates are read-only")
	}

	err = database.Conn().Transaction(func(tx *gorm.DB) error {
		var locked models.Canvas
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", canvas.ID).
			Where("organization_id = ?", organizationID).
			Where("deleted_at IS NULL").
			First(&locked).Error; err != nil {
			return err
		}
		activeResources, err := models.CountActiveManagedResourcesForCanvasInTransaction(tx, canvas.ID)
		if err != nil {
			return err
		}
		if activeResources > 0 {
			return status.Errorf(codes.FailedPrecondition, "canvas has %d active managed resources; delete or force-forget them first", activeResources)
		}
		return locked.SoftDeleteInTransaction(tx)
	})
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		log.Errorf("failed to delete canvas %s: %v", canvas.ID.String(), err)
		return nil, status.Error(codes.Internal, "failed to delete canvas")
	}

	if err := messages.NewCanvasDeletedMessage(canvas.ID.String(), canvas.OrganizationID.String()).PublishDeleted(); err != nil {
		log.Errorf("failed to publish canvas deleted RabbitMQ message: %v", err)
	}

	return &pb.DeleteCanvasResponse{}, nil
}
