package ngap

import (
	"testing"

	"github.com/stretchr/testify/require"

	amf_context "github.com/free5gc/amf/internal/context"
	ngapType "github.com/free5gc/ngap/ie"
	ngapMessage "github.com/free5gc/ngap/message"
	"github.com/free5gc/openapi/models"
)

// The source releases the UE (UE CONTEXT RELEASE REQUEST and COMPLETE) while
// the target is still preparing; the target's acknowledge arrives late. The
// AMF must not send a HANDOVER COMMAND for the released source; it cancels the
// handover at the SMF and releases the target (TS 23.502 4.11.1.2.3).
func TestHandoverRequestAcknowledgeAfterSourceRelease(t *testing.T) {
	e := newHoTestEnv(t, true)
	dispatchMain(e.srcRan, &ngapMessage.UEContextReleaseRequest{
		AMFUENGAPID: &ngapType.AMFUENGAPID{Value: e.src.AmfUeNgapId},
		RANUENGAPID: &ngapType.RANUENGAPID{Value: e.src.RanUeNgapId},
		Cause: &ngapType.Cause{Choice: &ngapType.CauseRadioNetwork{
			Value: ngapType.CauseRadioNetworkPresentUserInactivity,
		}},
	}, nil)
	dispatchMain(e.srcRan, &ngapMessage.UEContextReleaseComplete{
		AMFUENGAPID: &ngapType.AMFUENGAPID{Value: e.src.AmfUeNgapId},
		RANUENGAPID: &ngapType.RANUENGAPID{Value: e.src.RanUeNgapId},
	}, nil)
	hoRequireSent(t, e.srcConn, "source not released", "UEContextReleaseCommand")
	require.Nil(t, e.amfUe.RanUe[hoTestAnType])
	smfBefore := len(e.smf.requests())

	dispatchMain(e.tgtRan, newHoTestAck(e, 7, 5), nil)

	hoRequireSent(t, e.srcConn, "message sent to the released source", "UEContextReleaseCommand")
	hoStateCanceled := string(models.Smf_PDUSess_HoState_CANCELLED) //nolint:misspell // OpenAPI enum value
	e.smf.requireRequests(t, smfBefore, "SMF preparation not canceled", hoStateCanceled)
	require.Equal(t, amf_context.OnGoingProcedureNothing, e.amfUe.OnGoing(hoTestAnType).Procedure)
	hoRequireSent(t, e.tgtConn, "target resources not released", "HandoverRequest", "UEContextReleaseCommand")
	m, err := ngapMessage.Parse(e.tgtConn.MsgList[1])
	require.NoError(t, err)
	release, ok := m.(*ngapMessage.UEContextReleaseCommand)
	require.True(t, ok)
	require.Equal(t, &ngapType.Cause{Choice: &ngapType.CauseRadioNetwork{
		Value: ngapType.CauseRadioNetworkPresentHandoverCancelled,
	}}, release.Cause)

	require.NotPanics(t, func() {
		dispatchMain(e.tgtRan, &ngapMessage.UEContextReleaseComplete{
			AMFUENGAPID: &ngapType.AMFUENGAPID{Value: e.tgt.AmfUeNgapId},
			RANUENGAPID: &ngapType.RANUENGAPID{Value: 7},
		}, nil)
	})
	require.Nil(t, amf_context.GetSelf().RanUeFindByAmfUeNgapID(e.tgt.AmfUeNgapId), "target RanUe left behind")
	require.Nil(t, e.amfUe.RanUe[hoTestAnType])
}

// Control: with the source still serving, the acknowledge is answered with a
// HANDOVER COMMAND and the notify completes the handover.
func TestHandoverRequestAcknowledgeWithServingSource(t *testing.T) {
	e := newHoTestEnv(t, true)
	dispatchMain(e.tgtRan, newHoTestAck(e, 7, 5), nil)
	hoRequireSent(t, e.srcConn, "no HANDOVER COMMAND", "HandoverCommand")
	e.smf.requireRequests(t, 0, "SMF not prepared", "PREPARING/HANDOVER_REQUIRED", "PREPARED/HANDOVER_REQ_ACK")

	dispatchMain(e.tgtRan, newHoTestNotify(e, 7), nil)
	require.Same(t, e.tgt, e.amfUe.RanUe[hoTestAnType])
}
