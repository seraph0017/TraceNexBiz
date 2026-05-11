package ticket

import (
	"context"
	"errors"
	"testing"
)

func TestCreate_RejectsBadCategory(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepo())
	_, err := svc.Create(context.Background(), CreateInput{
		OpenerType: "customer", OpenerID: 1, Subject: "x", Category: "bogus",
	})
	if !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("got %v", err)
	}
}

func TestCreate_AndReply_DriveStatusMachine(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepo())
	tk, err := svc.Create(context.Background(), CreateInput{
		OpenerType: "customer", OpenerID: 1, Subject: "billing q", Category: "billing", BodyMD: "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != "open" {
		t.Fatalf("status = %s", tk.Status)
	}
	// staff first reply → responding
	tk, _, err = svc.Reply(context.Background(), tk.ID, "staff", 99, "ack")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != "responding" {
		t.Fatalf("status = %s", tk.Status)
	}
}

func TestAssign_AndTransition(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepo())
	tk, _ := svc.Create(context.Background(), CreateInput{OpenerType: "partner", OpenerID: 5, Subject: "x", Category: "api"})
	tk, err := svc.Assign(context.Background(), tk.ID, 88)
	if err != nil {
		t.Fatal(err)
	}
	if *tk.AssignedTo != 88 || tk.Status != "assigned" {
		t.Fatalf("%+v", tk)
	}
	tk, err = svc.Transition(context.Background(), tk.ID, "responding")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != "responding" {
		t.Fatalf("status %s", tk.Status)
	}
	// invalid jump
	_, err = svc.Transition(context.Background(), tk.ID, "open")
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestAdminList_FiltersByStatusAndOpener(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepo())
	for i := 0; i < 3; i++ {
		_, _ = svc.Create(context.Background(), CreateInput{OpenerType: "customer", OpenerID: 1, Subject: "s", Category: "billing"})
	}
	_, _ = svc.Create(context.Background(), CreateInput{OpenerType: "partner", OpenerID: 2, Subject: "p", Category: "api"})
	list, total, err := svc.AdminList(context.Background(), ListQuery{OpenerType: "customer", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(list) != 3 {
		t.Fatalf("got %d/%d", len(list), total)
	}
}

func TestMyList_BOLAScope(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepo())
	_, _ = svc.Create(context.Background(), CreateInput{OpenerType: "customer", OpenerID: 1, Subject: "a", Category: "billing"})
	_, _ = svc.Create(context.Background(), CreateInput{OpenerType: "customer", OpenerID: 2, Subject: "b", Category: "billing"})
	list, _, _ := svc.MyList(context.Background(), "customer", 1, "", 10, 0)
	if len(list) != 1 || list[0].OpenerID != 1 {
		t.Fatalf("scope leaked: %+v", list)
	}
}

func TestContentReportCategory_Allowed(t *testing.T) {
	t.Parallel()
	if err := ValidateCategory("content_report"); err != nil {
		t.Fatal(err)
	}
}
