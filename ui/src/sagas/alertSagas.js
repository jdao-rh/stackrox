import { delay } from 'redux-saga';
import { all, take, takeLatest, call, cancel, fork, put, select } from 'redux-saga/effects';
import queryString from 'query-string';

import * as service from 'services/AlertsService';
import { actions, types } from 'reducers/alerts';
import { types as locationActionTypes } from 'reducers/routes';
import { selectors } from 'reducers';

const dashboardPath = '/main/dashboard';
const violationsPath = '/main/violations';

function* getAlert({ params: alertId }) {
    try {
        const result = yield call(service.fetchAlert, alertId);
        yield put(actions.fetchAlert.success(result.response));
    } catch (error) {
        yield put(actions.fetchAlert.failure(error, alertId));
    }
}

function* getAlertNumsByPolicy(filters) {
    try {
        const result = yield call(service.fetchAlertNumsByPolicy, filters);
        yield put(actions.fetchAlertNumsByPolicy.success(result.response));
    } catch (error) {
        yield put(actions.fetchAlertNumsByPolicy.failure(error, filters));
    }
}

function* getAlertCountsByPolicyCategories() {
    try {
        const result = yield call(service.fetchAlertCountsByPolicyCategories);
        yield put(actions.fetchAlertCountsByPolicyCategories.success(result.response));
    } catch (error) {
        yield put(actions.fetchAlertCountsByPolicyCategories.failure(error));
    }
}

function* getAlertCountsByCluster() {
    try {
        const result = yield call(service.fetchAlertCountsByCluster);
        yield put(actions.fetchAlertCountsByCluster.success(result.response));
    } catch (error) {
        yield put(actions.fetchAlertCountsByCluster.failure(error));
    }
}

function* getAlertsByTimeseries() {
    try {
        const result = yield call(service.fetchAlertsByTimeseries);
        yield put(actions.fetchAlertsByTimeseries.success(result.response));
    } catch (error) {
        yield put(actions.fetchAlertsByTimeseries.failure(error));
    }
}

function* getAlertsByPolicy() {
    const policyId = yield select(selectors.getSelectedViolatedPolicyId);
    if (!policyId) return;
    try {
        const result = yield call(service.fetchAlertsByPolicy, policyId);
        yield put(actions.fetchAlertsByPolicy.success(result.response, policyId));
    } catch (error) {
        yield put(actions.fetchAlertsByPolicy.failure(error, policyId));
    }
}

function* pollAlertsByPolicy(filters) {
    while (true) {
        let failsCount = 0;
        try {
            yield all([call(getAlertNumsByPolicy, filters), call(getAlertsByPolicy)]);
            failsCount = 0;
        } catch (err) {
            console.error('Error during alerts polling', err);
            failsCount += 1;
            if (failsCount === 2) {
                // complain when retry didn't help
                yield put(actions.fetchAlertsByPolicy.failure('Cannot reach the server.'));
            }
        }
        yield delay(5000); // poll every 5 sec
    }
}

function* watchViolationsLocation() {
    let pollTask;
    while (true) {
        // it's a tricky/hack-y behavior here when deployment whitelisting happens: UI closes the dialog,
        // it causes location to update and therefore we're re-fetching everything for alerts
        const action = yield take(locationActionTypes.LOCATION_CHANGE);
        const { payload: location } = action;

        if (pollTask) yield cancel(pollTask); // cancel polling in any case
        if (location && location.pathname && location.pathname.startsWith(violationsPath)) {
            pollTask = yield fork(pollAlertsByPolicy, queryString.parse(location.search));
        }
    }
}

function* watchDashboardLocation() {
    while (true) {
        const action = yield take(locationActionTypes.LOCATION_CHANGE);
        const { payload: location } = action;

        if (location && location.pathname && location.pathname.startsWith(dashboardPath)) {
            yield all([
                fork(getAlertCountsByPolicyCategories),
                fork(getAlertCountsByCluster),
                fork(getAlertsByTimeseries)
            ]);
        }
    }
}

function* watchAlertRequest() {
    yield takeLatest(types.FETCH_ALERT.REQUEST, getAlert);
}

function* watchSelectedViolatedPolicy() {
    yield takeLatest(types.SELECT_VIOLATED_POLICY, getAlertsByPolicy);
}

export default function* alerts() {
    yield all([
        fork(watchViolationsLocation),
        fork(watchDashboardLocation),
        fork(watchSelectedViolatedPolicy),
        fork(watchAlertRequest)
    ]);
}
