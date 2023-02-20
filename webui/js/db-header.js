"use strict";

function _slicedToArray(arr, i) { return _arrayWithHoles(arr) || _iterableToArrayLimit(arr, i) || _unsupportedIterableToArray(arr, i) || _nonIterableRest(); }
function _nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }
function _unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return _arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return _arrayLikeToArray(o, minLen); }
function _arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) arr2[i] = arr[i]; return arr2; }
function _iterableToArrayLimit(arr, i) { var _i = null == arr ? null : "undefined" != typeof Symbol && arr[Symbol.iterator] || arr["@@iterator"]; if (null != _i) { var _s, _e, _x, _r, _arr = [], _n = !0, _d = !1; try { if (_x = (_i = _i.call(arr)).next, 0 === i) { if (Object(_i) !== _i) return; _n = !1; } else for (; !(_n = (_s = _x.call(_i)).done) && (_arr.push(_s.value), _arr.length !== i); _n = !0); } catch (err) { _d = !0, _e = err; } finally { try { if (!_n && null != _i["return"] && (_r = _i["return"](), Object(_r) !== _r)) return; } finally { if (_d) throw _e; } } return _arr; } }
function _arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }
function ToggleButton(_ref) {
  var icon = _ref.icon,
    textSet = _ref.textSet,
    textUnset = _ref.textUnset,
    redirectUrl = _ref.redirectUrl,
    updateUrl = _ref.updateUrl,
    pageUrl = _ref.pageUrl,
    isSet = _ref.isSet,
    count = _ref.count,
    cyToggle = _ref.cyToggle,
    cyPage = _ref.cyPage,
    disabled = _ref.disabled;
  var _React$useState = React.useState(isSet),
    _React$useState2 = _slicedToArray(_React$useState, 2),
    state = _React$useState2[0],
    setState = _React$useState2[1];
  var _React$useState3 = React.useState(count),
    _React$useState4 = _slicedToArray(_React$useState3, 2),
    number = _React$useState4[0],
    setNumber = _React$useState4[1];
  function gotoPage() {
    window.location = pageUrl;
  }
  function toggleState() {
    if (authInfo.loggedIn !== true) {
      // User needs to be logged in
      lock.show();
      return;
    }
    if (redirectUrl !== undefined) {
      window.location = redirectUrl;
      return;
    }

    // Retrieve the branch list for the newly selected database
    fetch(updateUrl).then(function (response) {
      return response.text();
    }).then(function (text) {
      // Update button text
      setState(!state);

      // Update displayed count
      setNumber(text);
    });
  }
  return /*#__PURE__*/React.createElement("div", {
    "class": "btn-group"
  }, /*#__PURE__*/React.createElement("button", {
    type: "button",
    "class": "btn btn-default",
    onClick: toggleState,
    "data-cy": cyToggle,
    disabled: disabled
  }, /*#__PURE__*/React.createElement("i", {
    "class": "fa " + icon
  }), " ", state ? textSet : textUnset), /*#__PURE__*/React.createElement("button", {
    type: "button",
    "class": "btn btn-default",
    onClick: gotoPage,
    "data-cy": cyPage
  }, number));
}
function DbHeader() {
  var forkedFrom = null;
  if (meta.forkOwner) {
    forkedFrom = /*#__PURE__*/React.createElement("div", {
      style: {
        fontSize: "small"
      }
    }, "forked from ", /*#__PURE__*/React.createElement("a", {
      href: "/" + meta.forkOwner
    }, meta.forkOwner), " /\xA0", meta.forkDeleted ? "deleted database" : /*#__PURE__*/React.createElement("a", {
      href: "/" + meta.forkOwner + "/" + meta.forkDatabase
    }, meta.forkDatabase));
  }
  var settings = null;
  if (authInfo.loggedIn) {
    settings = /*#__PURE__*/React.createElement("label", {
      id: "settings",
      "class": meta.pageSection == "db_settings" ? "dbMenuLinkActive" : "dbMenuLink"
    }, /*#__PURE__*/React.createElement("a", {
      href: "/settings/" + meta.owner + "/" + meta.database,
      "class": "blackLink",
      title: "Settings",
      "data-cy": "settingslink"
    }, /*#__PURE__*/React.createElement("i", {
      "class": "fa fa-cog"
    }), " Settings"));
  }
  var publicString = "Private";
  if (meta.publicDb) {
    publicString = "Public";
  }
  var visibility = null;
  if (meta.owner == authInfo.loggedInUser) {
    visibility = /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("b", null, "Visibility:"), " ", /*#__PURE__*/React.createElement("a", {
      "class": "blackLink",
      href: "/settings/" + meta.owner + "/" + meta.database,
      "data-cy": "vis"
    }, publicString));
  } else {
    visibility = /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("b", null, "Visibility:"), " ", /*#__PURE__*/React.createElement("span", {
      "data-cy": "vis"
    }, publicString));
  }
  var licence = null;
  if (meta.owner == authInfo.loggedInUser) {
    licence = /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("b", null, "Licence:"), " ", /*#__PURE__*/React.createElement("a", {
      "class": "blackLink",
      href: "/settings/" + meta.owner + "/" + meta.database,
      "data-cy": "lic"
    }, meta.licence));
  } else {
    if (meta.licenceUrl != "") {
      licence = /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("b", null, "Licence:"), " ", /*#__PURE__*/React.createElement("a", {
        "class": "blackLink",
        href: meta.licenceURL,
        "data-cy": "licurl"
      }, meta.licence));
    } else {
      licence = /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("b", null, "Licence:"), " ", /*#__PURE__*/React.createElement("span", {
        "data-cy": "licurl"
      }, meta.licence));
    }
  }
  return /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("div", {
    "class": "row"
  }, /*#__PURE__*/React.createElement("div", {
    "class": "col-md-12"
  }, /*#__PURE__*/React.createElement("h2", {
    id: "viewdb",
    style: {
      marginTop: "10px"
    }
  }, /*#__PURE__*/React.createElement("div", {
    "class": "pull-left"
  }, /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("a", {
    "class": "blackLink",
    href: "/" + meta.owner,
    "data-cy": "headerownerlnk"
  }, meta.owner), " /\xA0", /*#__PURE__*/React.createElement("a", {
    "class": "blackLink",
    href: "/" + meta.owner + "/" + meta.database,
    "data-cy": "headerdblnk"
  }, meta.database)), forkedFrom), /*#__PURE__*/React.createElement("div", {
    "class": "pull-right"
  }, /*#__PURE__*/React.createElement(ToggleButton, {
    icon: "fa-eye",
    textSet: "Unwatch",
    textUnset: "Watch",
    updateUrl: "/x/watch/" + meta.owner + "/" + meta.database,
    pageUrl: "/watchers/" + meta.owner + "/" + meta.database,
    isSet: meta.isWatching,
    count: meta.numWatchers,
    cyToggle: "watcherstogglebtn",
    cyPage: "watcherspagebtn"
  }), "\xA0", /*#__PURE__*/React.createElement(ToggleButton, {
    icon: "fa-star",
    textSet: "Unstar",
    textUnset: "Star",
    updateUrl: "/x/star/" + meta.owner + "/" + meta.database,
    pageUrl: "/stars/" + meta.owner + "/" + meta.database,
    isSet: meta.isStarred,
    count: meta.numStars,
    cyToggle: "starstogglebtn",
    cyPage: "starspagebtn"
  }), "\xA0", /*#__PURE__*/React.createElement(ToggleButton, {
    icon: "fa-sitemap",
    textSet: "Fork",
    textUnset: "Fork",
    redirectUrl: "/x/forkdb/" + meta.owner + "/" + meta.database + "?commit=" + meta.commitID,
    pageUrl: "/forks/" + meta.owner + "/" + meta.database,
    isSet: false,
    count: meta.numForks,
    cyToggle: "forksbtn",
    cyPage: "forkspagebtn",
    disabled: meta.owner == authInfo.loggedInUser
  }))))), /*#__PURE__*/React.createElement("div", {
    "class": "row",
    style: {
      paddingBottom: "5px",
      paddingTop: "10px"
    }
  }, /*#__PURE__*/React.createElement("div", {
    "class": "col-md-6"
  }, /*#__PURE__*/React.createElement("label", {
    id: "viewdata",
    "class": meta.pageSection == "db_data" ? "dbMenuLinkActive" : "dbMenuLink"
  }, /*#__PURE__*/React.createElement("a", {
    href: "/" + meta.owner + "/" + meta.database,
    "class": "blackLink",
    title: "Data",
    "data-cy": "datalink"
  }, /*#__PURE__*/React.createElement("i", {
    "class": "fa fa-database"
  }), " Data")), "\xA0 \xA0 \xA0", /*#__PURE__*/React.createElement("label", {
    id: "viewvis",
    "class": meta.pageSection == "db_vis" ? "dbMenuLinkActive" : "dbMenuLink"
  }, /*#__PURE__*/React.createElement("a", {
    href: "/vis/" + meta.owner + "/" + meta.database,
    "class": "blackLink",
    title: "Visualise",
    "data-cy": "vislink"
  }, /*#__PURE__*/React.createElement("i", {
    "class": "fa fa-bar-chart"
  }), " Visualise")), "\xA0 \xA0 \xA0", /*#__PURE__*/React.createElement("label", {
    id: "viewdiscuss",
    "class": meta.pageSection == "db_disc" ? "dbMenuLinkActive" : "dbMenuLink"
  }, /*#__PURE__*/React.createElement("a", {
    href: "/discuss/" + meta.owner + "/" + meta.database,
    "class": "blackLink",
    title: "Discussions",
    "data-cy": "discusslink"
  }, /*#__PURE__*/React.createElement("i", {
    "class": "fa fa-commenting"
  }), " Discussions:"), " ", meta.numDiscussions), "\xA0 \xA0 \xA0", /*#__PURE__*/React.createElement("label", {
    id: "viewmrs",
    "class": meta.pageSection == "db_merge" ? "dbMenuLinkActive" : "dbMenuLink"
  }, /*#__PURE__*/React.createElement("a", {
    href: "/merge/" + meta.owner + "/" + meta.database,
    "class": "blackLink",
    title: "Merge Requests",
    "data-cy": "mrlink"
  }, /*#__PURE__*/React.createElement("i", {
    "class": "fa fa-clone"
  }), " Merge Requests:"), " ", meta.numMRs), "\xA0 \xA0 \xA0", settings), /*#__PURE__*/React.createElement("div", {
    "class": "col-md-6"
  }, /*#__PURE__*/React.createElement("div", {
    "class": "pull-right"
  }, visibility, " \xA0", /*#__PURE__*/React.createElement("b", null, "Last Commit:"), " ", meta.commitID.substring(0, 8), " (", getTimePeriod(meta.repoModified, false), ") \xA0", licence, " \xA0", /*#__PURE__*/React.createElement("b", null, "Size:"), " ", /*#__PURE__*/React.createElement("span", {
    "data-cy": "size"
  }, Math.round(meta.size / 1024).toLocaleString(), " KB")))));
}
var rootNode = document.getElementById('db-header-root');
var root = ReactDOM.createRoot(rootNode);
root.render(React.createElement(DbHeader));