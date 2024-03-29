const React = require("react");
const ReactDOM = require("react-dom");

export default function AboutPage() {
	return (<>
		<h3 data-cy="aboutus">About Us</h3>

		<h4><a id="whatis"></a>What is DBHub.io</h4>

		<p>
			We - <a href="https://github.com/orgs/sqlitebrowser/people" rel="noopener noreferrer external">the people</a> behind&nbsp;
			<a href="https://sqlitebrowser.org" rel="noopener noreferrer external">DB Browser for SQLite</a> (DB4S) - are adding an
			optional "Cloud" storage service for SQLite databases.
		</p>

		<h4><a id="why"></a>Why?</h4>

		<p>It's pretty simple. :)</p>

		<p>
			We've been putting time into DB4S for years, it's fairly popular (150k+ downloads every month), and
			we'd like to be able <br/> to both work on it full time &amp; have actual lives.
		</p>

		<p>If we can generate sufficient ongoing revenue to make this all work, then yay, everyone wins! :)</p>

		<h4><a id="howopen"></a>How much is Open Source?</h4>

		<p>
			<i><b>Everything</b></i> is open source (<a href="https://www.gnu.org/licenses/agpl-3.0.html" rel="noopener noreferrer external">AGPL3</a> or later).
		</p>

		<p>
			Nothing held back, no "open core", etc.  This is real, actual, proper, Open Source.  Not the fake variety. :)
		</p>

		<h4><a id="intendedfeatures"></a>Features we intend to include</h4>

		<p>
			Most of these are still "in development" or will come along later, they're all on our definite To Do list:
		</p>

		<ul className="list-unstyled">
			<li className="p-2"><i className="fa fa-database fa-lg"></i> Basic send/receive of SQLite databases from DB4S (SQLite Browser)</li>
			<li className="p-2"><i className="fa fa-arrow-circle-right fa-lg"></i> Management of uploaded files. eg delete, rename, etc</li>
			<li className="p-2"><i className="fa fa-calendar fa-lg"></i> Online viewer/editor, with access controls</li>
			<li className="p-2"><i className="fa fa-sitemap fa-lg"></i> Teams + public/private databases</li>
			<li className="p-2"><i className="fa fa-balance-scale fa-lg"></i> Versioning for databases + basic "diff" support</li>
			<li className="p-2"><i className="fa fa-list-ol fa-lg"></i> An "Issues" section (trouble ticketing) for your databases</li>
			<li className="p-2"><i className="fa fa-indent fa-lg"></i> Forks, Pull Requests, Merging as per GitHub model</li>
			<li className="p-2"><i className="fa fa-arrows-alt fa-lg"></i> Branches, as per the git concept</li>
			<li className="p-2"><i className="fa fa-file-text-o fa-lg"></i> Support for email replys to comments, for Issues/PR's/etc</li>
			<li className="p-2"><i className="fa fa-file-image-o fa-lg"></i> Drag &amp; drop image support for Issues/PR's/etc</li>
			<li className="p-2"><i className="fa fa-exchange fa-lg"></i> An API, so people can query/update their database from "<a href="https://serverless.com" rel="noopener noreferrer external">Serverless</a>" applications</li>
		</ul>

		<h4><a id="pricing"></a>How much will it cost?</h4>

		<p>Completely undetermined at this stage. ;)</p>

		<p>
			The concept GitHub uses for pricing - free for public stuff, $ for private - is
			appealing, but <b><i>may</i></b> not work for databases.  At least initially everything
			will be free, which should give us a start towards understanding data usage patterns.
		</p>

		<p>With that we can develop a workable model, though it may take a few iterations.</p>
	</>);
}
