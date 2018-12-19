import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { components } from 'react-select';
import * as Icon from 'react-feather';

import { Creatable } from 'Components/ReactSelect';

const placeholderCreator = placeholderText => () => (
    <span className="text-base-500 flex h-full items-center pointer-events-none">
        <span className="font-600 absolute">{placeholderText}</span>
    </span>
);

const Option = ({ children, ...rest }) => (
    <components.Option {...rest}>
        <div className="flex">
            <span className="search-option-categories px-2 text-sm">{children}</span>
        </div>
    </components.Option>
);

const ValueContainer = ({ ...props }) => (
    <React.Fragment>
        <span className="text-base-500 flex h-full items-center pl-2 pr-1 pointer-events-none">
            <Icon.Search color="currentColor" size={18} />
        </span>
        <components.ValueContainer {...props} />
    </React.Fragment>
);

const MultiValue = props => (
    <components.MultiValue
        {...props}
        className={
            props.data.type === 'categoryOption'
                ? 'bg-primary-200 border border-primary-300 text-primary-700'
                : 'bg-base-200 border border-base-300 text-base-600'
        }
    />
);

const noOptionsMessage = () => null;

class SearchInput extends Component {
    static propTypes = {
        className: PropTypes.string,
        placeholder: PropTypes.string,
        searchOptions: PropTypes.arrayOf(PropTypes.object),
        searchModifiers: PropTypes.arrayOf(PropTypes.object),
        searchSuggestions: PropTypes.arrayOf(PropTypes.object),
        setSearchOptions: PropTypes.func.isRequired,
        setSearchSuggestions: PropTypes.func.isRequired,
        onSearch: PropTypes.func,
        isGlobal: PropTypes.bool,
        defaultOption: PropTypes.shape({
            value: PropTypes.string,
            label: PropTypes.string,
            category: PropTypes.string
        })
    };

    static defaultProps = {
        placeholder: 'Add one or more resource filters',
        className: '',
        searchOptions: [],
        searchModifiers: [],
        searchSuggestions: [],
        onSearch: null,
        isGlobal: false,
        defaultOption: null
    };

    componentWillUnmount() {
        if (!this.props.isGlobal) this.props.setSearchOptions([]);
    }

    setOptions = (_, searchOptions) => {
        // If there is a default option and one search value given, then potentially prepend the default search option
        if (
            this.props.defaultOption &&
            searchOptions.length === 1 &&
            !this.props.searchModifiers.find(x => x.value === searchOptions[0].value)
        ) {
            searchOptions.unshift(this.props.defaultOption);
        }
        this.props.setSearchOptions(searchOptions);
        if (this.props.onSearch) this.props.onSearch(searchOptions);
    };

    getSuggestions = () => {
        const { searchOptions, searchModifiers } = this.props;
        let searchSuggestions = [];
        if (searchOptions.length && searchOptions[searchOptions.length - 1].type) {
            // If you previously typed a search modifier (Cluster:, Deployment Name:, etc.) then don't show any search suggestions
            searchSuggestions = [];
        } else {
            searchSuggestions = searchModifiers;
        }
        return searchSuggestions;
    };

    render() {
        const Placeholder = placeholderCreator(this.props.placeholder);
        const { searchOptions, className } = this.props;
        const hideDropdown = this.getSuggestions().length ? '' : 'hide-dropdown';
        const props = {
            className: `${className} ${hideDropdown}`,
            components: { ValueContainer, Option, Placeholder, MultiValue },
            options: this.getSuggestions(),
            optionValue: searchOptions,
            onChange: this.setOptions,
            isMulti: true,
            noOptionsMessage,
            isValidNewOption: inputValue => {
                if (!this.props.defaultOption && this.props.searchOptions.length === 0) {
                    return false;
                }
                return inputValue;
            }
        };
        return <Creatable {...props} components={{ ...props.components }} autoFocus />;
    }
}

export default SearchInput;
