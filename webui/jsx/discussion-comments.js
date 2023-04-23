const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";
import { confirmAlert } from "react-confirm-alert";
import "react-confirm-alert/src/react-confirm-alert.css";

function DiscussionTopComment({setStatusMessage, setStatusMessageColour}) {
	const [discTitle, setDiscTitle] = React.useState(discussionData.title);
	const [discBody, setDiscBody] = React.useState(discussionData.body);
	const [savedDiscTitle, setSavedDiscTitle] = React.useState(discussionData.title);
	const [discBodyRendered, setDiscBodyRendered] = React.useState(discussionData.body_rendered);
	const [editDiscussion, setEditDiscussion] = React.useState(false);

	// Send the updated discussion values to the server
	function updateDiscussion() {
		// Retrieve text from the discussion edit area
		const title = document.getElementById("disctitle").value;
		const txt = document.getElementById("disctext").value;

		// Send the new discussion text to the server
		fetch("/x/updatediscuss/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"disctext": encodeURIComponent(txt),
				"disctitle": encodeURIComponent(title),
				"discid": discussionData.disc_id,
				"dbname": meta.database,
				"username": meta.owner,
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Updating the discussion text succeeded, so switch back to the static view
			response.text().then(text => {
				setDiscBody(txt);
				setDiscBodyRendered(text);
				setSavedDiscTitle(title);
				setEditDiscussion(false);

				// Clear any previous error message
				setStatusMessageColour("green");
				setStatusMessage("");
			});
		})
		.catch((error) => {
			setStatusMessageColour("red");
			setStatusMessage("Updating discussion failed");
		});
	}

	// Merges the request
	function mergeRequest() {
		fetch("/x/mergerequest/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"mrid": discussionData.disc_id,
				"dbname": meta.database,
				"username": meta.owner,
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Merging the MR succeeded, so update the status (we cheat for now by just reloading the page)
			window.location = "/merge/" + meta.owner + "/" + meta.database + "?id=" + discussionData.disc_id;
		})
		.catch((error) => {
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Merging failed: " + text);
			});
		});
	}

	// Closes the merge request
	function closeRequest() {
		// Send the comment text to the server
		fetch("/x/createcomment/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"comtext": "",
				"close": true,
				"discid": discussionData.disc_id,
				"dbname": meta.database,
				"username": meta.owner,
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Closing the MR succeeded, so update the status (we cheat for now by just reloading the page)
			window.location = "/merge/" + meta.owner + "/" + meta.database + "?id=" + discussionData.disc_id;
		})
		.catch((error) => {
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Closing merge request failed: " + text);
			});
		});
	}

	// Prepare rendered rows for commit table for merge requests
	const commitRows = (mrData === null ? [] : mrData.commitList.map(function(row) {
		return (
			<tr>
				<td>
					<a href={"/" + row.author_username} className="blackLink">
						{row.author_avatar !== "" ? <img src={row.author_avatar} height="18" width="18" style={{border: "1px solid #8c8c8c"}} /> : null}&nbsp;
						{row.author_name}
					</a>
				</td>
				<td>
					<a className="blackLink" href={discussionData.open ? ("/diffs/" + discussionData.mr_details.source_owner + "/" + discussionData.mr_details.source_database_name + "?commit_a=" + row.parent + "&commit_b=" + row.id) : null}>
						{row.id.substring(0, 8)}
					</a>
				</td>
				<td>
					{row.message === "" ? <span className="text-muted">This commit has no commit message</span> : row.message}
					{row.licence_change !== "" ? <span className="text-danger">{row.licence_change}</span> : null}
				</td>
				<td>
					<span title={new Date(row.timestamp).toLocaleString()}>{getTimePeriod(row.timestamp, true)}</span>
				</td>
			</tr>
		);
	}));

	return (
		<div className="panel panel-default">
			<div className="panel-heading">
				<h4>
					<a className="blackLink" href={"/" + discussionData.creator}>{discussionData.avatar_url !== "" ? <img style={{border: "1px solid #8c8c8c"}} src={discussionData.avatar_url} height="30" width="30" /> : null}</a>&nbsp;
					{editDiscussion ? <input className="form-control" id="disctitle" value={discTitle} onChange={(e) => setDiscTitle(e.target.value)} /> : <strong>{savedDiscTitle}<span className="text-muted"> #{discussionData.disc_id}</span></strong>}
					{editDiscussion === false && (discussionData.creator === authInfo.loggedInUser || meta.owner === authInfo.loggedInUser) ? <span className="pull-right" style={{fontSize: "medium"}}><a className="blackLink" onClick={() => setEditDiscussion(true)}><i className="fa fa-pencil fa-fw"></i></a></span> : null}
				</h4>
				{discussionData.open ?
					<span className={"label label-success"}>
						<i className={"fa fa-dot-circle-o"}></i> Open
					</span>
				:
					<span className={"label label-danger"}>
						<i className={"fa fa-check-square-o"}></i> Closed
					</span>
				}
				&nbsp;
				{mrData !== null ?
					<span>
						Opened <span title={new Date(discussionData.creation_date).toLocaleString()} className="text-info">{getTimePeriod(discussionData.creation_date, true)}</span>: <a href={"/" + discussionData.creator}>{discussionData.creator}</a>
						{discussionData.open ? " wants to merge " : " requested a merge from "}
                                        	<a href={mrData.sourceDbOk ? ("/" + discussionData.mr_details.source_owner + "/" + discussionData.mr_details.source_database_name) : null}>{discussionData.mr_details.source_owner + "/" + discussionData.mr_details.source_database_name}</a>
						&nbsp;branch&nbsp;
                                        	<a href={mrData.sourceDbOk && mrData.sourceBranchOk ? ("/commits/" + discussionData.mr_details.source_owner + "/" + discussionData.mr_details.source_database_name + "?branch=" + discussionData.mr_details.source_branch) : null}>{discussionData.mr_details.source_branch}</a>
						&nbsp;into&nbsp;
                                        	<a href={mrData.destBranchNameOk ? ("/commits/" + meta.owner + "/" + meta.database + "?branch=" + discussionData.mr_details.destination_branch) : null}>{discussionData.mr_details.destination_branch}</a>
					</span>
				:
					<span>
						Opened <span title={new Date(discussionData.creation_date).toLocaleString()} className="text-info">{getTimePeriod(discussionData.creation_date, true)}</span> by <a href={"/" + discussionData.creator}>{discussionData.creator}</a>
					</span>
				}
			</div>
			<div className="panel-body">
				{editDiscussion ? <>
					<MarkdownEditor editorId="disctext" rows={10} initialValue={discBody} />
					<input type="submit" className="btn btn-success" value="Save" style={{marginTop: "1em"}} onClick={() => updateDiscussion()} />&nbsp;
					<input type="submit" className="btn btn-default" value="Cancel" style={{marginTop: "1em"}} onClick={() => setEditDiscussion(false)} />
				</> :
					<span dangerouslySetInnerHTML={{__html: discBodyRendered}} />
				}
			</div>
			{mrData !== null ? <>
				<div className="panel-heading">
					<h4>Commit list</h4>
					{discussionData.open ? <a href={"/diffs/" + discussionData.mr_details.source_owner + "/" + discussionData.mr_details.source_database_name + "?commit_a=" + mrData.commitList[mrData.commitList.length - 1].parent + "&commit_b=" + mrData.commitList[0].id}>View changes</a> : null}
				</div>
				<table className="table">
					<thead>
						<tr><th>Author</th><th>Commit ID</th><th>Commit message</th><th>Date</th></tr>
					</thead>
					<tbody>
						{commitRows}
					</tbody>
				</table>
				{discussionData.mr_details.state !== 1 && (discussionData.creator === authInfo.loggedInUser || meta.owner === authInfo.loggedInUser) ?
					<div className="panel-body">
						{discussionData.open === true && meta.owner === authInfo.loggedInUser && mrData.destBranchNameOk === true && mrData.destBranchUsable === true ? <><input className="btn btn-success" value="Merge the request" onClick={() => mergeRequest()} />&nbsp;</> : null}
						{discussionData.creator === authInfo.loggedInUser || meta.owner === authInfo.loggedInUser ? <input className="btn btn-default" value={discussionData.open ? "Close without merging" : "Reopen merge request"} onClick={() => closeRequest()} /> : null}
					</div>
				: null}
			</> : null}
		</div>
	);
}

function DiscussionComment({commentData, setStatusMessage, setStatusMessageColour}) {
	const [commentBody, setCommentBody] = React.useState(commentData.body);
	const [commentBodyRendered, setCommentBodyRendered] = React.useState(commentData.body_rendered);
	const [editComment, setEditComment] = React.useState(false);

	// Send the updated comment text to the server
	function updateComment() {
		const txt = document.getElementById("com" + commentData.com_id).value;

		fetch("/x/updatecomment/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"comtext": encodeURIComponent(txt),
				"comid": commentData.com_id,
				"discid": discussionData.disc_id,
				"dbname": meta.database,
				"username": meta.owner,
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Updating the comment text succeeded, so display it in the list
			response.text().then(text => {
				setCommentBody(txt);
				setCommentBodyRendered(text);
				setEditComment(false);

				// Clear any previous error message
				setStatusMessageColour("green");
				setStatusMessage("");
			});
		})
		.catch((error) => {
			setStatusMessageColour("red");
			setStatusMessage("Updating comment failed");
		});
	}

	// Deletes a comment
	function deleteComment() {
		confirmAlert({
			title: "Confirm delete",
			message: "Are you sure you want to delete this comment?",
			buttons: [
				{
					label: 'Yes',
					onClick: () => {
						fetch("/x/deletecomment/", {
							method: "post",
							headers: {
								"Content-Type": "application/x-www-form-urlencoded"
							},
							body: new URLSearchParams({
								"comid": commentData.com_id,
								"discid": discussionData.disc_id,
								"dbname": meta.database,
								"username": meta.owner,
							}),
						}).then((response) => {
							if (!response.ok) {
								return Promise.reject(response);
							}

							// Deleting the comment succeeded, so reload the page
							window.location = "/" + (mrData === null ? "discuss" : "merge") + "/" + meta.owner + "/" + meta.database + "?id=" + discussionData.disc_id;
						})
						.catch((error) => {
							// Deleting the comment failed, so display an error message
							setStatusMessageColour("red");
							setStatusMessage("Deleting comment failed");
						});
					}
				},
				{
					label: 'No'
				}
			]
		});
	}

	// Special comment type: Discussion closed?
	if (commentData.entry_type === "cls") {
		return (
			<div className="text-center">
				<i className="fa fa-ban text-danger fa-2g"></i>&nbsp;
				<a className="blackLink" href={"/" + commentData.commenter}>{commentData.commenter}</a>&nbsp;
				<span className="text-info">{discussionData.mr_details === 1 ? "merged" : "closed"} this</span>&nbsp;
				<span title={new Date(commentData.creation_date).toLocaleString()}>{getTimePeriod(commentData.creation_date, true)}</span>.
			</div>
		);
	}

	// Special comment type: Discussion reopened?
	if (commentData.entry_type === "rop") {
		return (
			<div className="text-center">
				<i className="fa fa-recycle text-success fa-2g"></i>&nbsp;
				<a className="blackLink" href={"/" + commentData.commenter}>{commentData.commenter}</a>&nbsp;
				<span className="text-info">reopened this</span>&nbsp;
				<span title={new Date(commentData.creation_date).toLocaleString()}>{getTimePeriod(commentData.creation_date, true)}</span>.
			</div>
		);
	}

	// Regular comment
	return (
		<div className="panel panel-default">
			<div className="panel-heading">
				<a className="blackLink" href={"/" + commentData.commenter}>{commentData.avatar_url !== "" ? <img style={{border: "1px solid #8c8c8c"}} src={commentData.avatar_url} height="30" width="30" /> : null}</a>&nbsp;
				<a className="blackLink" href={"/" + commentData.commenter}>{commentData.commenter}</a>&nbsp;
				<a name={"c" + commentData.com_id} href={"#c" + commentData.com_id} className="blackLink">commented</a>&nbsp;
				<span title={new Date(commentData.creation_date).toLocaleString()} className="text-info">{getTimePeriod(commentData.creation_date, true)}</span>
				{commentData.commenter === authInfo.loggedInUser || meta.owner === authInfo.loggedInUser ? (
					<span className="pull-right" style={{fontSize: "medium"}}>
						<a className="blackLink" onClick={() => setEditComment(!editComment)}><i className="fa fa-pencil fa-fw"></i></a>
						<a className="blackLink" onClick={() => deleteComment()}><i className="fa fa-trash-o fa-fw"></i></a>
					</span>
				) : null}
			</div>
			<div className="panel-body">
				{editComment ? <>
					<MarkdownEditor editorId={"com" + commentData.com_id} rows={10} initialValue={commentBody} />
					<input type="submit" className="btn btn-success" value="Save" style={{marginTop: "1em"}} onClick={() => updateComment()} />&nbsp;
					<input type="submit" className="btn btn-default" value="Cancel" style={{marginTop: "1em"}} onClick={() => setEditComment(false)} />
				</> :
					<span dangerouslySetInnerHTML={{__html: commentBodyRendered}} />
				}
			</div>
		</div>
	);
}

export default function DiscussionComments() {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");
	const [addCommentText, setAddCommentText] = React.useState("");

	// Switch to the create discussion page
	function createDiscussion() {
		if (authInfo.loggedInUser) {
			window.location = "/" + (mrData === null ? "creatediscuss" : "compare") + "/" + meta.owner + "/" + meta.database;
		} else {
			// User needs to be logged in
			lock.show();
		}
	}

	// Displays the login dialog
	function signIn() {
		lock.show();
	}

	// Add a comment to the discussion
	function addComment(alsoClose) {
		const txt = document.getElementById("comtext").value;

		fetch("/x/createcomment/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"comtext": encodeURIComponent(txt),
				"close": alsoClose,
				"discid": discussionData.disc_id,
				"dbname": meta.database,
				"username": meta.owner,
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Adding the comment succeeded, so display it in the list (we cheat for now by just reloading the page)
			window.location = "/" + (mrData === null ? "discuss" : "merge") + "/" + meta.owner + "/" + meta.database + "?id=" + discussionData.disc_id;
		})
		.catch((error) => {
			setStatusMessageColour("red");
			setStatusMessage("Adding comment failed");
		});
	}

	// Render all the discussion comments
	const comments = commentsData !== null ? commentsData.map(c => DiscussionComment({commentData: c, setStatusMessage: setStatusMessage, setStatusMessageColour: setStatusMessageColour})) : [];

	// Decide on the text for the close button
	let closeButtonText = "";
	if (mrData === null) {
		if (addCommentText !== "" && discussionData.open === true) {
			closeButtonText = "Add comment and close discussion";
		} else if(addCommentText !== "" && discussionData.open === false) {
			closeButtonText = "Add comment and reopen discussion";
		} else if (addCommentText === "" && discussionData.open === true) {
			closeButtonText = "Close discussion";
		} else if(addCommentText === "" && discussionData.open === false) {
			closeButtonText = "Reopen discussion";
		}
	} else {
		if (addCommentText !== "" && discussionData.open === true) {
			closeButtonText = "Add comment and close without merging";
		} else if(addCommentText !== "" && discussionData.open === false) {
			closeButtonText = "Add comment and reopen merge request";
		} else if (addCommentText === "" && discussionData.open === true) {
			closeButtonText = "Close without merging";
		} else if(addCommentText === "" && discussionData.open === false) {
			closeButtonText = "Reopen merge request";
		}
	}

	return (<>
		<div className="row">
			<div className="col-md-12 text-center" style={{marginBottom: "1em"}}>
				<button className="btn btn-success" onClick={() => createDiscussion()}>{mrData === null ? "Start a new discussion" : "New Merge Request"}</button>
			</div>
		</div>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center">
					<div style={{paddingBottom: "1em"}}>
						<h4 style={{color: statusMessageColour}}>{statusMessage}</h4>
					</div>
				</div>
			</div>
		) : null}
		{mrData !== null && mrData.licenceWarning !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center">
					<div style={{paddingBottom: "8px"}}>
						<h4 className="text-warning">{mrData.licenceWarning}</h4>
					</div>
				</div>
			</div>
		) : null}
		<DiscussionTopComment setStatusMessage={setStatusMessage} setStatusMessageColour={setStatusMessageColour} />
		{comments}
		{authInfo.loggedInUser ? (
			<div className="panel panel-default">
				<div className="panel-body">
					<MarkdownEditor editorId={"comtext"} rows={10} placeholder="Add your comment here..." onChange={v => setAddCommentText(v)} />
					<input type="submit" className="btn btn-success" value="Add comment" style={{marginTop: "1em"}} onClick={() => addComment(false)} />&nbsp;
					{discussionData.creator === authInfo.loggedInUser || meta.owner === authInfo.loggedInUser ? <input type="submit" className="btn btn-default" value={closeButtonText} style={{marginTop: "1em"}} onClick={() => addComment(true)} /> : null}
				</div>
			</div>
		) : (
			<div className="panel panel-default text-center">
				<div className="panel-body">
					<a href="#/" onClick={() => signIn()}>Sign in</a> to join the discussion
				</div>
			</div>
		)}
	</>);
}
