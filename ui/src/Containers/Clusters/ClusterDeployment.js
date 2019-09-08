import React from 'react';
import PropTypes from 'prop-types';
import * as Icon from 'react-feather';
import { ClipLoader } from 'react-spinners';

import ToggleSwitch from 'Components/ToggleSwitch';

const baseClass = 'py-6 border-b border-base-300 border-solid';

const ClusterDeploymentPage = ({
    onFileDownload,
    clusterCheckedIn,
    editing,
    createUpgraderSA,
    toggleSA
}) => (
    <div className="w-full">
        <div className="px-4">
            {editing && clusterCheckedIn && (
                <div className="pt-3 font-700">Dynamic configuration has been synced to Sensor</div>
            )}
            <div className={baseClass}>
                1){' '}
                {editing
                    ? 'If static configuration was updated, download the new configuration files'
                    : 'Download the required configuration files'}
                <div className="mb-4 flex items-center py-2 border-b-2 border-base-300">
                    <label
                        htmlFor="createUpgraderSA"
                        className="py-2 text-base-600 font-700 flex w-full"
                    >
                        Include creation of Service Account for Auto-Upgrades:
                    </label>
                    <ToggleSwitch
                        id="createUpgraderSA"
                        name="createUpgraderSA"
                        toggleHandler={toggleSA}
                        enabled={createUpgraderSA}
                    />
                </div>
                <div className="flex justify-center p-3">
                    <button
                        type="button"
                        className="download uppercase text-primary-600 p-2 text-center text-sm border border-solid bg-primary-200 border-primary-300 hover:bg-primary-100"
                        onClick={onFileDownload}
                        tabIndex="-1"
                    >
                        <span className="pr-2">Download YAML file and keys</span>
                        <Icon.Download className="h-3 w-3" />
                    </button>
                </div>
                <div className="text-xs text-center text-base-600">
                    * You may modify the YAML to suit your environment as needed
                </div>
            </div>
            <div className={baseClass}>
                2) Use the deploy script inside the zip file to launch the sensor in your
                environment
            </div>
        </div>
        {(!editing || !clusterCheckedIn) && (
            <div className="flex flex-col text-primary-500 p-4">
                {clusterCheckedIn ? (
                    <div className="flex flex text-success-600 bg-success-200 border border-solid border-success-400 p-4 items-center">
                        <div className="flex-1 text-center">
                            <Icon.CheckCircle />
                        </div>
                        <div className="flex-3 pl-2">
                            Success! The cluster has been recognized properly by StackRox.
                        </div>
                    </div>
                ) : (
                    <div className="flex flex text-primary-600 bg-primary-200 border border-solid border-primary-400 p-4 items-center">
                        <div className="text-center px-4">
                            <ClipLoader color="currentColor" loading size={20} />
                        </div>
                        <div className="flex-3 pl-2">
                            Waiting for the cluster to check-in successfully...
                        </div>
                    </div>
                )}
            </div>
        )}
    </div>
);

ClusterDeploymentPage.propTypes = {
    onFileDownload: PropTypes.func.isRequired,
    clusterCheckedIn: PropTypes.bool.isRequired,
    editing: PropTypes.bool.isRequired,
    createUpgraderSA: PropTypes.bool.isRequired,
    toggleSA: PropTypes.func.isRequired
};

export default ClusterDeploymentPage;
