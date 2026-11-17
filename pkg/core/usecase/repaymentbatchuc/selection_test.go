package repaymentbatchuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func monthlyTestInstallment(order int,status model.InstallmentStatus,due time.Time) model.Installment {
	return model.Installment{
		ID:model.InstallmentID(uuid.New()),
		InstallmentOrder:order,
		Status:status,
		DueAt:due,
	}
}

func TestRepaymentSelectionRequiresAllOverdueProducts(t *testing.T) {
	loc,err:=time.LoadLocation("Asia/Tehran")
	if err!=nil {t.Fatal(err)}
	due:=time.Date(2026,time.November,5,23,59,59,0,loc)
	monthly:=monthlyTestInstallment(1,model.DueInstallmentStatus,due)
	monthly.GracePeriodDays=0
	four:=monthlyTestInstallment(2,model.OverdueInstallmentStatus,due)
	selection:=map[model.InstallmentID]struct{}{monthly.ID:{}}
	now:=time.Date(2026,time.November,6,0,0,0,0,loc)
	if err:=requireAllOverdueInstallments(selection,[]model.Installment{monthly,four},now,loc);err==nil{
		t.Fatal("repayment skipped another overdue installment")
	}
	selection[four.ID]=struct{}{}
	if err:=requireAllOverdueInstallments(selection,[]model.Installment{monthly,four},now,loc);err!=nil{t.Fatal(err)}
	before:=time.Date(2026,time.November,5,20,0,0,0,loc)
	delete(selection,monthly.ID)
	if err:=requireAllOverdueInstallments(selection,[]model.Installment{monthly,four},before,loc);err!=nil{
		t.Fatal("not-yet-overdue monthly installment was forced into payment: ",err)
	}
}

func TestRepaymentSelectionRequiresConsecutiveUnpaidInstallments(t *testing.T){
	now:=time.Date(2026,time.November,5,23,59,59,0,time.UTC)
	first:=monthlyTestInstallment(1,model.PaidInstallmentStatus,now)
	second:=monthlyTestInstallment(2,model.DueInstallmentStatus,now)
	third:=monthlyTestInstallment(3,model.ScheduledInstallmentStatus,now)
	fourth:=monthlyTestInstallment(4,model.ScheduledInstallmentStatus,now)
	all:=[]model.Installment{first,second,third,fourth}
	skip:=map[model.InstallmentID]struct{}{third.ID:{}}
	if err:=requireConsecutiveInstallments(skip,all);err==nil{t.Fatal("later installment skipped unpaid second installment")}
	valid:=map[model.InstallmentID]struct{}{second.ID:{},third.ID:{}}
	if err:=requireConsecutiveInstallments(valid,all);err!=nil{t.Fatal(err)}
}
