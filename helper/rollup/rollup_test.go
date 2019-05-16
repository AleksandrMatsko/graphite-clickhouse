package rollup

import (
	"fmt"
	"testing"
	"time"

	"github.com/lomik/graphite-clickhouse/helper/point"
)

func TestParseXML(t *testing.T) {
	config := `
<graphite_rollup>
 	<pattern>
 		<regexp>click_cost</regexp>
 		<function>any</function>
 		<retention>
 			<age>0</age>
 			<precision>3600</precision>
 		</retention>
 		<retention>
 			<age>86400</age>
 			<precision>60</precision>
 		</retention>
 	</pattern>
 	<default>
 		<function>max</function>
 		<retention>
 			<age>0</age>
 			<precision>60</precision>
 		</retention>
 		<retention>
 			<age>3600</age>
 			<precision>300</precision>
 		</retention>
 		<retention>
 			<age>86400</age>
 			<precision>3600</precision>
 		</retention>
 	</default>
</graphite_rollup>
`

	r, err := ParseXML([]byte(config))
	if err != nil {
		t.Fatal(err)
	}

	if r.Pattern[0].Retention[1].Age != 86400 {
		t.FailNow()
	}

	if r.Default.Retention[2].Precision != 3600 {
		t.FailNow()
	}
}

func TestParseClickhouseXML(t *testing.T) {
	config := `
<yandex>
	<graphite_rollup>
		<pattern>
			<regexp>click_cost</regexp>
			<function>any</function>
			<retention>
				<age>0</age>
				<precision>3600</precision>
			</retention>
			<retention>
				<age>86400</age>
				<precision>60</precision>
			</retention>
		</pattern>
		<default>
			<function>max</function>
			<retention>
				<age>0</age>
				<precision>60</precision>
			</retention>
			<retention>
				<age>3600</age>
				<precision>300</precision>
			</retention>
			<retention>
				<age>86400</age>
				<precision>3600</precision>
			</retention>
		</default>
	</graphite_rollup>
</yandex>
`

	r, err := ParseXML([]byte(config))
	t.Logf("%+v", r)
	if err != nil {
		t.Fatal(err)
	}

	if r.Pattern[0].Retention[1].Age != 86400 {
		t.FailNow()
	}

	if r.Default.Retention[2].Precision != 3600 {
		t.FailNow()
	}
}

func TestMetricPrecision(t *testing.T) {
	tests := [][2][]point.Point{
		{
			{ // in
				{MetricID: 1, Time: 1478025152, Value: 3},
				{MetricID: 1, Time: 1478025154, Value: 2},
				{MetricID: 1, Time: 1478025255, Value: 1},
			},
			{ // out
				{MetricID: 1, Time: 1478025120, Value: 5},
				{MetricID: 1, Time: 1478025240, Value: 1},
			},
		},
	}

	for _, test := range tests {
		result := doMetricPrecision(test[0], 60, AggrMap["sum"])
		point.AssertListEq(t, test[1], result)
	}
}

func TestMetricStep(t *testing.T) {
	config := `
<graphite_rollup>
 	<pattern>
 		<regexp>^metric\.</regexp>
 		<function>any</function>
 		<retention>
 			<age>0</age>
 			<precision>1</precision>
 		</retention>
 		<retention>
 			<age>3600</age>
 			<precision>10</precision>
 		</retention>
 	</pattern>
 	<default>
 		<function>max</function>
 		<retention>
 			<age>0</age>
 			<precision>60</precision>
 		</retention>
 		<retention>
 			<age>3600</age>
 			<precision>300</precision>
 		</retention>
 		<retention>
 			<age>86400</age>
 			<precision>3600</precision>
 		</retention>
 	</default>
</graphite_rollup>
`
	r, err := ParseXML([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	now := uint32(time.Now().Unix())

	tests := []struct {
		name         string
		from         uint32
		expectedStep uint32
	}{
		{"metric.foo.first-retention", now - 500, 1},
		{"metric.foo.second-retention", now - 3600, 10},
		{"foo.bar.default-first-retention", now - 500, 60},
		{"foo.bar.default-second-retention", now - 3700, 300},
		{"foo.bar.default-last-retention", now - 87000, 3600},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("metric=%v (from=now-%v)", test.name, now-test.from), func(t *testing.T) {
			step, err := r.Step(test.name, test.from)
			if err != nil {
				t.Fatalf("error=%s", err.Error())
			}
			if step != test.expectedStep {
				t.Fatalf("metric=%v (from=now-%v), expected step=%v, actual step=%v", test.name, now-test.from, test.expectedStep, step)
			}
		})
	}
}
