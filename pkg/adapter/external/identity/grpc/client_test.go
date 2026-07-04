package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/parstis/bnpl-backend/gen/proto/identity/v1"
	cbnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extidentity "github.com/parstis/bnpl-backend/pkg/core/external/identity"
)

type fakeIdentityService struct {
	resolveResp *identityv1.ResolveSubjectByMobileResponse
	resolveErr  error
	resolveReq  *identityv1.ResolveSubjectByMobileRequest
}

func (f *fakeIdentityService) ResolveSubjectByMobile(
	_ context.Context,
	in *identityv1.ResolveSubjectByMobileRequest,
	_ ...grpcclient.CallOption,
) (*identityv1.ResolveSubjectByMobileResponse, error) {
	f.resolveReq = in

	return f.resolveResp, f.resolveErr
}

func newTestClient(t *testing.T, fake *fakeIdentityService) *Client {
	t.Helper()

	client, err := newClient(fake, time.Second, cbnoop.New())
	require.NoError(t, err)

	return client
}

func TestNew_RequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := New(nil, time.Second, cbnoop.New())
	require.Error(t, err)

	_, err = newClient(&fakeIdentityService{}, time.Second, nil)
	require.Error(t, err)
}

func TestResolveByMobile_MapsIndividualActive(t *testing.T) {
	t.Parallel()

	subject := uuid.New()
	fake := &fakeIdentityService{
		resolveResp: &identityv1.ResolveSubjectByMobileResponse{
			SubjectId:     subject.String(),
			SubjectType:   "individual",
			SubjectStatus: "active",
		},
	}

	out, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)
	require.Equal(t, subject, out.IdentitySubjectID.UUID())
	require.Equal(t, extidentity.IndividualSubjectType, out.SubjectType)
	require.Equal(t, extidentity.ActiveSubjectStatus, out.SubjectStatus)
	require.Equal(t, "+989121234567", fake.resolveReq.GetMobileNumber())
}

func TestResolveByMobile_MapsSuspendedAndDeleted(t *testing.T) {
	t.Parallel()

	for _, statusValue := range []string{"suspended", "deleted"} {
		t.Run(statusValue, func(t *testing.T) {
			t.Parallel()

			subject := uuid.New()
			fake := &fakeIdentityService{
				resolveResp: &identityv1.ResolveSubjectByMobileResponse{
					SubjectId:     subject.String(),
					SubjectType:   "individual",
					SubjectStatus: statusValue,
				},
			}

			out, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
				MobileE164: "+989121234567",
			})
			require.NoError(t, err)
			require.Equal(t, extidentity.SubjectStatus(statusValue), out.SubjectStatus)
		})
	}
}

func TestResolveByMobile_MapsGroup(t *testing.T) {
	t.Parallel()

	fake := &fakeIdentityService{
		resolveResp: &identityv1.ResolveSubjectByMobileResponse{
			SubjectId:     uuid.New().String(),
			SubjectType:   "group",
			SubjectStatus: "active",
		},
	}

	out, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)
	require.Equal(t, extidentity.GroupSubjectType, out.SubjectType)
}

func TestResolveByMobile_UnknownTypeOrStatusIsProtocolError(t *testing.T) {
	t.Parallel()

	t.Run("unknown type", func(t *testing.T) {
		t.Parallel()

		fake := &fakeIdentityService{
			resolveResp: &identityv1.ResolveSubjectByMobileResponse{
				SubjectId:     uuid.New().String(),
				SubjectType:   "robot",
				SubjectStatus: "active",
			},
		}

		_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
			MobileE164: "+989121234567",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "subject_type")
		require.NotContains(t, err.Error(), "+98912")
	})

	t.Run("unknown status", func(t *testing.T) {
		t.Parallel()

		fake := &fakeIdentityService{
			resolveResp: &identityv1.ResolveSubjectByMobileResponse{
				SubjectId:     uuid.New().String(),
				SubjectType:   "individual",
				SubjectStatus: "pending",
			},
		}

		_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
			MobileE164: "+989121234567",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "subject_status")
	})
}

func TestResolveByMobile_DoesNotTrimWireValues(t *testing.T) {
	t.Parallel()

	t.Run("padded subject type", func(t *testing.T) {
		t.Parallel()

		fake := &fakeIdentityService{
			resolveResp: &identityv1.ResolveSubjectByMobileResponse{
				SubjectId:     uuid.New().String(),
				SubjectType:   " individual ",
				SubjectStatus: "active",
			},
		}

		_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
			MobileE164: "+989121234567",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "subject_type")
	})

	t.Run("padded subject status", func(t *testing.T) {
		t.Parallel()

		fake := &fakeIdentityService{
			resolveResp: &identityv1.ResolveSubjectByMobileResponse{
				SubjectId:     uuid.New().String(),
				SubjectType:   "individual",
				SubjectStatus: " active ",
			},
		}

		_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
			MobileE164: "+989121234567",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "subject_status")
	})

	t.Run("padded subject id", func(t *testing.T) {
		t.Parallel()

		fake := &fakeIdentityService{
			resolveResp: &identityv1.ResolveSubjectByMobileResponse{
				SubjectId:     " " + uuid.New().String() + " ",
				SubjectType:   "individual",
				SubjectStatus: "active",
			},
		}

		_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
			MobileE164: "+989121234567",
		})
		require.Error(t, err)
	})
}

func TestResolveByMobile_InvalidSubjectID(t *testing.T) {
	t.Parallel()

	t.Run("invalid uuid", func(t *testing.T) {
		t.Parallel()

		fake := &fakeIdentityService{
			resolveResp: &identityv1.ResolveSubjectByMobileResponse{
				SubjectId:     "not-a-uuid",
				SubjectType:   "individual",
				SubjectStatus: "active",
			},
		}

		_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
			MobileE164: "+989121234567",
		})
		require.Error(t, err)
	})

	t.Run("zero uuid", func(t *testing.T) {
		t.Parallel()

		fake := &fakeIdentityService{
			resolveResp: &identityv1.ResolveSubjectByMobileResponse{
				SubjectId:     uuid.Nil.String(),
				SubjectType:   "individual",
				SubjectStatus: "active",
			},
		}

		_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
			MobileE164: "+989121234567",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "zero")
	})
}

func TestResolveByMobile_ErrorMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		code    codes.Code
		message string
		want    cerr.Code
	}{
		{
			name:    "not found",
			code:    codes.NotFound,
			message: "no subject for +989121234567",
			want:    cerr.CodeResourceNotFound,
		},
		{
			name:    "unavailable",
			code:    codes.Unavailable,
			message: "failed resolving +989121234567",
			want:    cerr.CodeServiceUnavailable,
		},
		{
			name:    "deadline",
			code:    codes.DeadlineExceeded,
			message: "timed out for +989121234567",
			want:    cerr.CodeServiceUnavailable,
		},
		{
			name:    "permission",
			code:    codes.PermissionDenied,
			message: "caller cannot resolve +989121234567",
			want:    cerr.CodeInternalError,
		},
		{
			name:    "exhausted",
			code:    codes.ResourceExhausted,
			message: "rate limited for +989121234567",
			want:    cerr.CodeTooManyRequests,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeIdentityService{resolveErr: status.Error(tt.code, tt.message)}
			_, err := newTestClient(t, fake).ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
				MobileE164: "+989121234567",
			})

			var coded *cerr.Error
			require.ErrorAs(t, err, &coded)
			require.Equal(t, tt.want, coded.Code)
			require.NotContains(t, err.Error(), "+989121234567")
			require.NotContains(t, err.Error(), tt.message)
			require.Contains(t, err.Error(), "grpc_code=")
		})
	}
}
