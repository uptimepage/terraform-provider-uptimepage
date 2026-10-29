package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

const groupUUID = "0b5c1a4e-6d2f-4c8a-9e3b-7f1d2a6c4e90"

func TestGroupToModel_ReadsBackInTheAttributeTheConfigUses(t *testing.T) {
	server := &client.Target{GroupName: strptr("example.com DNS"), GroupID: strptr(groupUUID)}
	cases := []struct {
		name             string
		prior            targetModel
		wantName, wantID string // "" = null
	}{
		{"by id", targetModel{GroupName: types.StringNull(), GroupID: types.StringValue(groupUUID)}, "", groupUUID},
		{"by name", targetModel{GroupName: types.StringValue("example.com DNS"), GroupID: types.StringNull()}, "example.com DNS", ""},
		{"import reads by name", targetModel{GroupName: types.StringNull(), GroupID: types.StringNull()}, "example.com DNS", ""},
		{"untrimmed name kept", targetModel{GroupName: types.StringValue(" example.com DNS "), GroupID: types.StringNull()}, " example.com DNS ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, id := groupToModel(tc.prior, server)
			if got := name.ValueString(); got != tc.wantName || name.IsNull() != (tc.wantName == "") {
				t.Errorf("group_name = %v, want %q", name, tc.wantName)
			}
			if got := id.ValueString(); got != tc.wantID || id.IsNull() != (tc.wantID == "") {
				t.Errorf("group_id = %v, want %q", id, tc.wantID)
			}
		})
	}
}

// A monitor managed by id that someone moved out of its group in the app
// must plan the move back.
func TestGroupToModel_ByIDSeesTheMonitorLeave(t *testing.T) {
	_, id := groupToModel(targetModel{GroupName: types.StringNull(), GroupID: types.StringValue(groupUUID)}, &client.Target{})
	if !id.IsNull() {
		t.Errorf("group_id = %v, want null", id)
	}
}

func TestTargetToUpdate_CarriesGroupID(t *testing.T) {
	m := targetModel{
		Name:      types.StringValue("ns"),
		Tags:      types.SetNull(types.StringType),
		GroupName: types.StringNull(),
		GroupID:   types.StringValue(groupUUID),
		Check:     checkModel{Type: types.StringValue(client.CheckTypeManual)},
	}
	out, d := m.toUpdate(context.Background(), m)
	if d.HasError() {
		t.Fatalf("toUpdate: %v", d)
	}
	if out.GroupID == nil || *out.GroupID != groupUUID || out.GroupName != nil {
		t.Errorf("group = id %v, name %v", out.GroupID, out.GroupName)
	}
}

// validateResourceConfig runs a resource config through the provider server's
// validation, attribute validators included.
func validateResourceConfig(t *testing.T, typeName string, objType tftypes.Object, raw tftypes.Value) []*tfprotov6.Diagnostic {
	t.Helper()
	srv, err := testAccProtoV6["uptimepage"]()
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	cfg, err := tfprotov6.NewDynamicValue(objType, raw)
	if err != nil {
		t.Fatalf("dynamic value: %v", err)
	}
	resp, err := srv.ValidateResourceConfig(context.Background(), &tfprotov6.ValidateResourceConfigRequest{
		TypeName: typeName,
		Config:   &cfg,
	})
	if err != nil {
		t.Fatalf("ValidateResourceConfig: %v", err)
	}
	return resp.Diagnostics
}

func manualTargetConfig(objType tftypes.Object, groupName, groupID tftypes.Value) tftypes.Value {
	attrs := nullAttrs(objType)
	attrs["name"] = tftypes.NewValue(tftypes.String, "ns")
	attrs["interval"] = tftypes.NewValue(tftypes.Number, 60)
	checkType := objType.AttributeTypes["check"].(tftypes.Object)
	check := nullAttrs(checkType)
	check["type"] = tftypes.NewValue(tftypes.String, client.CheckTypeManual)
	attrs["check"] = tftypes.NewValue(checkType, check)
	attrs["group_name"] = groupName
	attrs["group_id"] = groupID
	return tftypes.NewValue(objType, attrs)
}

func TestTargetConfig_GroupIDConflictsWithGroupName(t *testing.T) {
	_, objType := resourceSchema(t, newTargetResource())
	name := tftypes.NewValue(tftypes.String, "dns")
	id := tftypes.NewValue(tftypes.String, groupUUID)
	none := tftypes.NewValue(tftypes.String, nil)

	diags := validateResourceConfig(t, "uptimepage_target", objType, manualTargetConfig(objType, name, id))
	if !hasErrorContaining(diags, "group_name") {
		t.Errorf("both set: want a conflict naming group_name, got %v", diags)
	}
	for label, raw := range map[string]tftypes.Value{
		"by name": manualTargetConfig(objType, name, none),
		"by id":   manualTargetConfig(objType, none, id),
	} {
		if diags := validateResourceConfig(t, "uptimepage_target", objType, raw); hasError(diags) {
			t.Errorf("%s: unexpected diagnostics %v", label, diags)
		}
	}
}

func hasError(diags []*tfprotov6.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			return true
		}
	}
	return false
}

func hasErrorContaining(diags []*tfprotov6.Diagnostic, s string) bool {
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError && strings.Contains(d.Summary+d.Detail, s) {
			return true
		}
	}
	return false
}

func TestMonitorGroupModel_RoundTrip(t *testing.T) {
	ctx := context.Background()
	tags, _ := types.SetValueFrom(ctx, types.StringType, []string{"dns", "p0"})
	plan := monitorGroupModel{
		Name:                 types.StringValue("example.com DNS "),
		AlertAsOne:           types.BoolValue(true),
		Alerts:               []alertModel{{ChannelID: types.StringValue("c1")}},
		RenotifyIntervalSecs: types.Int64Value(1800),
		AlertAgain:           types.BoolValue(false),
		Tags:                 tags,
	}
	in, d := plan.toInput(ctx)
	if d.HasError() {
		t.Fatalf("toInput: %v", d)
	}
	if !in.AlertAsOne || in.AlertAgain || in.RenotifyIntervalSecs != 1800 || len(in.Alerts) != 1 || len(in.Tags) != 2 {
		t.Errorf("toInput = %+v", in)
	}

	got, d := monitorGroupToModel(ctx, plan, &client.MonitorGroup{
		ID: groupUUID, Name: "example.com DNS", AlertAsOne: true,
		Alerts: in.Alerts, RenotifyIntervalSecs: 1800, Tags: in.Tags,
	})
	if d.HasError() {
		t.Fatalf("monitorGroupToModel: %v", d)
	}
	if got.Name.ValueString() != "example.com DNS " {
		t.Errorf("name = %q, want the user's spelling kept", got.Name.ValueString())
	}
	if !got.Tags.Equal(tags) || len(got.Alerts) != 1 || got.ID.ValueString() != groupUUID {
		t.Errorf("read back = %+v", got)
	}

	renamed, _ := monitorGroupToModel(ctx, plan, &client.MonitorGroup{ID: groupUUID, Name: "renamed in the app", Tags: []string{}})
	if renamed.Name.ValueString() != "renamed in the app" {
		t.Errorf("a rename in the app must show as drift, got %q", renamed.Name.ValueString())
	}
	if renamed.Alerts == nil || renamed.Tags.IsNull() {
		t.Errorf("empty read-back must be empty, not null: %+v", renamed)
	}
}

type fakeGroupListAPI struct{ groups []client.MonitorGroup }

func (f fakeGroupListAPI) ListGroups(context.Context) ([]client.MonitorGroup, error) {
	return f.groups, nil
}

func readMonitorGroupDataSource(t *testing.T, api monitorGroupListAPI, name string) (monitorGroupDataModel, *datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()
	d := &monitorGroupDataSource{api: api}
	var sresp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sresp)
	objType := sresp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	attrs := nullAttrs(objType)
	attrs["name"] = tftypes.NewValue(tftypes.String, name)
	resp := &datasource.ReadResponse{State: tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: sresp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: tftypes.NewValue(objType, attrs), Schema: sresp.Schema}}, resp)
	var out monitorGroupDataModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &out)...)
	}
	return out, resp
}

func TestMonitorGroupDataSource_FindsByName(t *testing.T) {
	api := fakeGroupListAPI{groups: []client.MonitorGroup{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "example.com DNS x"},
		{ID: groupUUID, Name: "example.com DNS", AlertAsOne: true, MemberCount: 14, Tags: []string{"dns"}},
	}}
	got, resp := readMonitorGroupDataSource(t, api, " example.com DNS")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	if got.ID.ValueString() != groupUUID || !got.AlertAsOne.ValueBool() || got.MemberCount.ValueInt64() != 14 {
		t.Errorf("read = %+v", got)
	}
	if got.Name.ValueString() != " example.com DNS" {
		t.Errorf("name = %q, want the config's value", got.Name.ValueString())
	}
}

func TestMonitorGroupDataSource_NamesAMissingGroup(t *testing.T) {
	_, resp := readMonitorGroupDataSource(t, fakeGroupListAPI{}, "nope")
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), `"nope"`) {
		t.Errorf("want a not-found error naming the group, got %v", resp.Diagnostics)
	}
}

func TestSectionToModel_KeepsTheHeadingSpelling(t *testing.T) {
	prior := statusPageSectionModel{PageID: types.StringValue("p1"), Heading: types.StringValue(" DNS ")}
	got := sectionToModel(prior, &client.StatusPageSection{ID: "s1", GroupID: groupUUID, Heading: strptr("DNS")})
	if got.Heading.ValueString() != " DNS " || got.PageID.ValueString() != "p1" || got.GroupID.ValueString() != groupUUID {
		t.Errorf("read = %+v", got)
	}
	imported := sectionToModel(statusPageSectionModel{PageID: types.StringValue("p1"), Heading: types.StringNull()},
		&client.StatusPageSection{ID: "s1", GroupID: groupUUID})
	if !imported.Heading.IsNull() {
		t.Errorf("heading = %v, want null for the group's name", imported.Heading)
	}
}

func TestSectionImport_TakesPageColonSection(t *testing.T) {
	ctx := context.Background()
	r := newStatusPageSectionResource()
	sch, objType := resourceSchema(t, r)
	for id, wantErr := range map[string]bool{"p1:s1": false, "p1/s1": true, ":s1": true, "p1:": true} {
		resp := &resource.ImportStateResponse{State: tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: sch}}
		r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
		if resp.Diagnostics.HasError() != wantErr {
			t.Errorf("%q: error = %v, want %v", id, resp.Diagnostics, wantErr)
			continue
		}
		if wantErr {
			continue
		}
		var got statusPageSectionModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
		if got.PageID.ValueString() != "p1" || got.ID.ValueString() != "s1" {
			t.Errorf("%q: state = %+v", id, got)
		}
	}
}

// fakeComponentAPI is one page's components; errors are keyed by method.
type fakeComponentAPI struct {
	rows    []client.StatusPageComponent
	errs    map[string]error
	updated *client.StatusPageComponentUpdate
}

func (f *fakeComponentAPI) AddComponent(context.Context, string, client.NewStatusPageComponent) error {
	return f.errs["add"]
}

func (f *fakeComponentAPI) ListComponents(context.Context, string) ([]client.StatusPageComponent, error) {
	return f.rows, f.errs["list"]
}

func (f *fakeComponentAPI) UpdateComponent(_ context.Context, _, _ string, in client.StatusPageComponentUpdate) error {
	f.updated = &in
	return f.errs["update"]
}

func (f *fakeComponentAPI) RemoveComponent(context.Context, string, string) error {
	return f.errs["remove"]
}

var alreadyOnPage = &client.APIError{Status: 409, Code: client.CodeComponentAlreadyOnPage}

func componentPlan() statusPageComponentModel {
	return statusPageComponentModel{
		PageID:     types.StringValue("p1"),
		TargetID:   types.StringValue("t1"),
		PublicName: types.StringValue("Nameservers"),
		SortOrder:  types.Int64Value(2),
	}
}

func TestComponentAdd_AdoptsTheRowASectionMade(t *testing.T) {
	api := &fakeComponentAPI{
		rows: []client.StatusPageComponent{{TargetID: "t1", SectionID: strptr("s1")}},
		errs: map[string]error{"add": alreadyOnPage},
	}
	r := &statusPageComponentResource{api: api}
	if err := r.add(context.Background(), "p1", componentPlan()); err != nil {
		t.Fatalf("add: %v", err)
	}
	if api.updated == nil || api.updated.PublicName == nil || *api.updated.PublicName != "Nameservers" ||
		api.updated.SortOrder == nil || *api.updated.SortOrder != 2 {
		t.Errorf("settings not applied to the adopted row: %+v", api.updated)
	}
}

// A section puts its members in order; adopting one only to name it must not
// move it.
func TestComponentAdd_AdoptionKeepsTheSectionsOrder(t *testing.T) {
	api := &fakeComponentAPI{
		rows: []client.StatusPageComponent{{TargetID: "t1", SectionID: strptr("s1"), SortOrder: 4}},
		errs: map[string]error{"add": alreadyOnPage},
	}
	plan := componentPlan()
	plan.SortOrder = types.Int64Unknown()
	if err := (&statusPageComponentResource{api: api}).add(context.Background(), "p1", plan); err != nil {
		t.Fatalf("add: %v", err)
	}
	if api.updated == nil || api.updated.SortOrder != nil {
		t.Errorf("sort_order sent on adopt: %+v", api.updated)
	}
}

func TestComponentAdd_SurfacesAFailedRead(t *testing.T) {
	broken := &client.APIError{Status: 502, Code: "UNKNOWN"}
	api := &fakeComponentAPI{errs: map[string]error{"add": alreadyOnPage, "list": broken}}
	if err := (&statusPageComponentResource{api: api}).add(context.Background(), "p1", componentPlan()); err != broken {
		t.Errorf("err = %v, want the list failure", err)
	}
}

func TestTagValidator_RefusesWhatTheAPIWouldTrim(t *testing.T) {
	for tag, ok := range map[string]bool{"dns": true, "eu west": true, " dns": false, "dns ": false, "": false, "\u00a0dns": false} {
		var resp validator.StringResponse
		trimmedValidator{}.ValidateString(context.Background(),
			validator.StringRequest{Path: path.Root("tags"), ConfigValue: types.StringValue(tag)}, &resp)
		if resp.Diagnostics.HasError() == ok {
			t.Errorf("%q: error = %v, want ok = %v", tag, resp.Diagnostics, ok)
		}
	}
}

// A row someone curated by hand is not the section's to hand over, so the
// conflict stands.
func TestComponentAdd_LeavesAHandRowsConflict(t *testing.T) {
	api := &fakeComponentAPI{
		rows: []client.StatusPageComponent{{TargetID: "t1"}},
		errs: map[string]error{"add": alreadyOnPage},
	}
	r := &statusPageComponentResource{api: api}
	if err := r.add(context.Background(), "p1", componentPlan()); !client.HasCode(err, client.CodeComponentAlreadyOnPage) {
		t.Errorf("err = %v, want the conflict", err)
	}
	if api.updated != nil {
		t.Errorf("a hand row was overwritten: %+v", api.updated)
	}
}

func TestComponentRemove_ResetsASectionMember(t *testing.T) {
	api := &fakeComponentAPI{errs: map[string]error{"remove": &client.APIError{Status: 409, Code: client.CodeSectionMember}}}
	r := &statusPageComponentResource{api: api}
	if err := r.remove(context.Background(), "p1", "t1"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if u := api.updated; u == nil || u.PublicName != nil || u.PublicDescription != nil || u.PublicGroup != nil || u.SortOrder != nil {
		t.Errorf("section member not reset to an empty update: %+v", u)
	}

	gone := &fakeComponentAPI{errs: map[string]error{"remove": &client.APIError{Status: 404, Code: "COMPONENT_NOT_FOUND"}}}
	if err := (&statusPageComponentResource{api: gone}).remove(context.Background(), "p1", "t1"); err != nil || gone.updated != nil {
		t.Errorf("a component already gone: err %v, updated %+v", err, gone.updated)
	}
}
