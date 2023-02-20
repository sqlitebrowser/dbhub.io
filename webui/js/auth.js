"use strict";

function Auth() {
  function login() {
    lock.show();
  }
  if (authInfo.loggedInUser) {
    var avatar = null;
    if (authInfo.avatarUrl) {
      avatar = /*#__PURE__*/React.createElement("img", {
        src: authInfo.avatarUrl,
        height: "18",
        width: "18",
        style: {
          border: "1px solid #8c8c8c"
        }
      });
    }
    var updates = null;
    if (authInfo.numStatusUpdates === 0) {
      updates = /*#__PURE__*/React.createElement("a", {
        href: "/updates",
        "class": "inBox",
        style: {
          verticalAlign: "middle"
        }
      }, /*#__PURE__*/React.createElement("i", {
        "class": "fa fa-inbox fa-fw",
        style: {
          fontSize: "large"
        }
      }));
    } else {
      updates = /*#__PURE__*/React.createElement("a", {
        href: "/updates",
        "class": "inBox",
        style: {
          verticalAlign: "middle",
          borderBottom: "1px grey dotted"
        }
      }, /*#__PURE__*/React.createElement("i", {
        "class": "fa fa-inbox fa-fw",
        style: {
          fontSize: "large"
        }
      }), authInfo.numStatusUpdates);
    }
    return /*#__PURE__*/React.createElement(React.Fragment, null, avatar, "\xA0", updates, "\xA0", /*#__PURE__*/React.createElement("a", {
      href: "/pref",
      style: {
        color: "black",
        verticalAlign: "middle"
      }
    }, "Settings"), " | ", /*#__PURE__*/React.createElement("a", {
      href: "/" + authInfo.loggedInUser,
      style: {
        color: "black",
        verticalAlign: "middle"
      }
    }, "Home"), " | ", /*#__PURE__*/React.createElement("a", {
      href: "/logout",
      style: {
        color: "black",
        verticalAlign: "middle"
      }
    }, "Log out"));
  } else {
    return /*#__PURE__*/React.createElement("a", {
      href: "",
      onClick: login,
      style: {
        color: "black"
      },
      "data-cy": "loginlnk"
    }, "Login / Register");
  }
}
var rootNode = document.getElementById('authcontrol');
var root = ReactDOM.createRoot(rootNode);
root.render(React.createElement(Auth));