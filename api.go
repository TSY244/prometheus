package prom

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Api struct {
	api          prometheusv1.API
	QueryTimeout time.Duration
}

func (a *Api) GetAllMetrics() ([]Metric, error) {
	lbls, err := a.createSeries("{__name__=~'.+'}")
	if err != nil {
		return nil, err
	}
	var ms []Metric
	for _, lbl := range lbls {
		if len(lbl) != 0 {
			m := make(Metric)
			for k, v := range lbl {
				m[LabelName(k)] = LabelValue(v)
			}
			ms = append(ms, m)
		}
	}
	return ms, nil
}

// Query Used to query the index according to the label and value conditions
func (a *Api) Query(indexName string, labelCondition LabelCondition, valueCondition ValueCondition) ([]Index, error) {
	return a.queryBase(labelCondition, valueCondition, func() (model.Value, error) {
		return a.createQuery(indexName)
	})
}

// QueryRange Used to query the index according to the label and value conditions
func (a *Api) QueryRange(queryStatement string, labelCondition LabelCondition, valueCondition ValueCondition, start, end time.Time, step time.Duration) ([]Index, error) {
	return a.queryBase(labelCondition, valueCondition, func() (model.Value, error) {
		return a.createRangeQuery(queryStatement, newRange(start, end, step))
	})
}

// queryBase Used to query the index according to the label and value conditions
func (a *Api) queryBase(labelCondition LabelCondition, valueCondition ValueCondition, queryFunc func() (model.Value, error)) ([]Index, error) {
	result, err := queryFunc()
	if err != nil {
		return nil, err
	}

	labelCondFuncs := translationLabelConditions(labelCondition)
	valueCondFunc, err := translationLabelCondition(valueCondition)
	if err != nil {
		return nil, err
	}

	var indexes []Index
	for _, r := range result.(model.Vector) {
		if isMatchingLabels(r.Metric, labelCondFuncs) && isMatchingValue(float64(r.Value), valueCondFunc) {
			indexes = append(indexes, createIndexFromMetric(r.Metric))
		}
	}

	return indexes, nil
}

// createRangeQuery Used to query the index
func (a *Api) createRangeQuery(queryStatement string, queryRange prometheusv1.Range) (model.Value, error) {
	return a.runQuery(func(ctx context.Context) (model.Value, []string, error) {
		return a.api.QueryRange(ctx, queryStatement, queryRange)
	})
}

// runQuery Used to query the index
func (a *Api) runQuery(queryFunc func(ctx context.Context) (model.Value, []string, error)) (model.Value, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.QueryTimeout)
	defer cancel()

	result, warnings, err := queryFunc(ctx)
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		return nil, errors.New("has warning")
	}
	if result == nil || len(result.(model.Vector)) == 0 {
		return nil, errors.New("no result")
	}
	return result, nil
}

// createSeries Used to query the index
func (a *Api) createSeries(queryStatement string) ([]model.LabelSet, error) {
	return a.runQuerySeries(func(ctx context.Context) ([]model.LabelSet, prometheusv1.Warnings, error) {
		return a.api.Series(ctx, []string{queryStatement}, time.Now().Add(-time.Hour), time.Now())
	})
}

// runQuerySeries is used to query the names of indicators
func (a *Api) runQuerySeries(queryFunc func(ctx context.Context) ([]model.LabelSet, prometheusv1.Warnings, error)) ([]model.LabelSet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), a.QueryTimeout)
	defer cancel()

	result, warnings, err := queryFunc(ctx)
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		return nil, errors.New("has warning")
	}
	if result == nil || len(result) == 0 {
		return nil, errors.New("no result")
	}
	return result, nil
}

// createQuery Used to query the index
func (a *Api) createQuery(indexName string) (model.Value, error) {
	return a.runQuery(func(ctx context.Context) (model.Value, []string, error) {
		return a.api.Query(ctx, indexName, time.Now())
	})
}

// newRange Used to create a range
func newRange(start, end time.Time, step time.Duration) prometheusv1.Range {
	return prometheusv1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}
}

// isMatchingLabels Used to determine whether the label meets the conditions
func isMatchingLabels(metric model.Metric, labelCondFuncs map[string]func(value string) bool) bool {
	if len(labelCondFuncs) != 0 {
		for k, v := range metric {
			for key, matchFunc := range labelCondFuncs {
				isMatch, err := MatchString(string(k), key)
				if err != nil {
					return false
				}
				if !isMatch {
					continue
				}
				if !matchFunc(string(v)) {
					return false
				}
			}
		}
	}
	return true
}

// isMatchingValue Used to determine whether the value meets the conditions
func isMatchingValue(value float64, valueCondFunc func(value float64) bool) bool {
	return valueCondFunc(value)
}

// createIndexFromMetric Convert Metric in the query results to Index
func createIndexFromMetric(metric model.Metric) Index {
	var index Index
	for k, v := range metric {
		index.Metrics = append(index.Metrics, Metric{LabelName(k): LabelValue(v)})
	}
	return index
}

// translationLabelConditions Analyze tag conditions and return the corresponding judgment function
func translationLabelConditions(condition LabelCondition) map[string]func(value string) bool {
	if len(condition) == 0 || condition == nil {
		return map[string]func(value string) bool{}
	}
	ret := make(map[string]func(value string) bool)
	for k, v := range condition {
		ret[k] = func(value string) bool {
			isMatch, err := MatchString(v, value)
			return isMatch && err == nil
		}
	}
	return ret
}

// translationLabelCondition Analyze numerical conditions and return the corresponding judgment function
func translationLabelCondition(valueCon ValueCondition) (func(value float64) bool, error) {
	if valueCon == "" {
		return func(value float64) bool {
			return true
		}, nil
	}
	symbol, condValue := parseCondition(string(valueCon))
	float64Value, err := strconv.ParseFloat(condValue, 64)
	if err != nil {
		return nil, err
	}
	switch symbol {
	case ">":
		return func(value float64) bool { return value > float64Value }, nil
	case "<":
		return func(value float64) bool { return value < float64Value }, nil
	case ">=":
		return func(value float64) bool { return value >= float64Value }, nil
	case "<=":
		return func(value float64) bool { return value <= float64Value }, nil
	case "==":
		return func(value float64) bool { return value == float64Value }, nil
	case "!=":
		return func(value float64) bool { return value != float64Value }, nil
	default:
		return func(value float64) bool { return true }, nil
	}
}

// parseCondition Parsing conditional strings
func parseCondition(condition string) (string, string) {
	t := strings.Split(condition, " ")
	if len(t) != 2 {
		return "", ""
	}
	return t[0], t[1]
}

func MatchString(findStr, matchStr string) (bool, error) {
	// check findStr if it is a regular expression
	r, err := regexp.Compile(findStr)
	if err != nil {
		return false, errors.New("findStr is not a regular expression")
	}
	return r.MatchString(matchStr), nil
}
