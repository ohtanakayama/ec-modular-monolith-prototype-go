package membersgrpc

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/ohtanakayama/ec-modular-monolith-prototype-go/gen/proto/members/v1"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/domain/member"
)

func toMemberPB(m *member.Member) *pb.Member {
	return &pb.Member{
		Id:        m.ID(),
		Email:     m.Email().String(),
		Name:      m.Name().String(),
		CreatedAt: timestamppb.New(m.CreatedAt()),
	}
}
