package def

import "testing"

func TestPlaceholderAPI(t *testing.T) {
	if Init() != nil {
		t.Fatalf("Init() should return nil placeholder")
	}
	if Query() != nil {
		t.Fatalf("Query() should return nil placeholder")
	}
	if Filter(true) != nil {
		t.Fatalf("Filter() should return nil placeholder")
	}
	if BindTable[int]("t") != nil {
		t.Fatalf("BindTable() should return nil placeholder")
	}
	if In(1, []int{1}) {
		t.Fatalf("In() should return false placeholder")
	}
	if IsNull(nil) {
		t.Fatalf("IsNull() should return false placeholder")
	}
	if IsNotNull(1) {
		t.Fatalf("IsNotNull() should return false placeholder")
	}
	if Column(1) != nil {
		t.Fatalf("Column() should return nil placeholder")
	}
	if Count[int64](1) != 0 {
		t.Fatalf("Count() should return zero value placeholder")
	}
	if Sum[float64](1.5) != 0 {
		t.Fatalf("Sum() should return zero value placeholder")
	}
	if Avg[float64](1.5) != 0 {
		t.Fatalf("Avg() should return zero value placeholder")
	}
	if Max[int](1) != 0 {
		t.Fatalf("Max() should return zero value placeholder")
	}
	if Min[int](1) != 0 {
		t.Fatalf("Min() should return zero value placeholder")
	}
	if Func[string]("UPPER", "x") != "" {
		t.Fatalf("Func() should return zero value placeholder")
	}
	if Create(1) != nil {
		t.Fatalf("Create() should return nil placeholder")
	}
	if Update(1, 2) != nil {
		t.Fatalf("Update() should return nil placeholder")
	}
	if Delete(1, 2) != nil {
		t.Fatalf("Delete() should return nil placeholder")
	}
	if Set(1, 2) != nil {
		t.Fatalf("Set() should return nil placeholder")
	}
	if Limit(1) != nil {
		t.Fatalf("Limit() should return nil placeholder")
	}
	if Offset(1) != nil {
		t.Fatalf("Offset() should return nil placeholder")
	}
}
