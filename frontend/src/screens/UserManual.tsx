const settingsRows = [
  ['Verification Hash', 'Adds a unique fingerprint for each file so copies can be checked later. BLAKE3 is the usual fast choice; choose Off for a quick list.'],
  ['Exclude hidden files', 'Leaves out hidden computer files that are usually not part of the records.'],
  ['Exclude common system files', 'Leaves out small files created by Windows or macOS, such as Thumbs.db or .DS_Store.'],
  ['Exclude extensions', 'Skips file types you do not want in the list. Enter examples like tmp, log, or cache.'],
  ['Create XLSX after scan', 'Also creates an Excel workbook after the scan finishes.'],
  ['Preserve leading zeros', 'Keeps numbers such as 001.001 from changing when the workbook opens in Excel.'],
  ['Delete CSV after XLSX', 'Keeps only the Excel workbook after it is made successfully.'],
  ['Folders only', 'Lists folders instead of files. Use Max depth when you only want the top few folder levels.'],
]

const agencyRows = [
  ['RC Series', 'Required for every agency-template scan. Enter it before starting the scan.'],
  ['RG, SG, Series', 'Use digits only. RG exports as 4 digits; SG and Series export as 3 digits, so 1 becomes 001 and 35 becomes 035.'],
  ['Department, Division, Section, Unit', 'Use these when the same value belongs on every row in the sheet.'],
  ['Begin Date, End Date', 'Use these when one date range applies to the whole sheet.'],
  ['Description, Comments', 'Use these for notes that should appear on every row.'],
  ['Location Override', 'Optional. Leave it blank unless every row should use one shared location.'],
  ['Material Type, Record Level', 'These start as Born Digital and Item. Change them only when the whole sheet needs different values.'],
]

const emailTypes = [
  '.dbx', '.eml', '.emlx', '.mbox', '.mbx', '.msg', '.olk14msgsource',
  '.olk15message', '.ost', '.pst', '.rge', '.tbb', '.wdseml',
]

export default function UserManual() {
  return (
    <div className="manual-page">
      <div className="screen-header">
        <h2 className="screen-title">User Manual</h2>
        <p className="screen-subtitle">Step-by-step help for staff using Content List Toolkit.</p>
      </div>

      <div className="manual-layout">
        <nav className="manual-toc" aria-label="User manual contents">
          <a href="#manual-quick-start">Quick Start</a>
          <a href="#manual-content-list">Content List</a>
          <a href="#manual-agency-template">Agency Template</a>
          <a href="#manual-email-copy">Email Copy</a>
          <a href="#manual-clone-compare">Clone Compare</a>
          <a href="#manual-outputs">Outputs</a>
          <a href="#manual-updates">Updates</a>
          <a href="#manual-troubleshooting">Troubleshooting</a>
        </nav>

        <article className="manual-document" aria-label="Content List Toolkit user manual">
          <section id="manual-quick-start" className="manual-section">
            <p className="manual-kicker">Getting Started</p>
            <h3>Quick start</h3>
            <ol className="manual-steps">
              <li>Choose the job you need from the sidebar.</li>
              <li>Use Browse to pick the source folder and the output folder.</li>
              <li>Choose only the options that apply to this job.</li>
              <li>Start the job and leave the app open until the done screen appears.</li>
              <li>Open the output folder from the done screen and review the files before sharing them.</li>
            </ol>
            <div className="manual-callout">
              The Light, Dark, and System buttons in the lower-left sidebar change this manual and the app together.
            </div>
          </section>

          <section id="manual-content-list" className="manual-section">
            <p className="manual-kicker">Workflow</p>
            <h3>Content List</h3>
            <p>
              Use Content List when you need a spreadsheet of files or folders. The app scans the source folder and
              writes one row for each item it includes. Turn on verification only when you need a file fingerprint for
              later checking.
            </p>
            <table className="manual-table">
              <thead>
                <tr><th>Setting</th><th>Use</th></tr>
              </thead>
              <tbody>
                {settingsRows.map(([setting, use]) => (
                  <tr key={setting}><td>{setting}</td><td>{use}</td></tr>
                ))}
              </tbody>
            </table>
          </section>

          <section id="manual-agency-template" className="manual-section">
            <p className="manual-kicker">Workflow</p>
            <h3>Agency Template</h3>
            <p>
              Use Agency Template when the spreadsheet must match the area agency headers. Fill in only the values that
              should be the same for the entire sheet. These fields start blank for every new scan so a value from an
              earlier job cannot be reused by mistake.
            </p>
            <table className="manual-table">
              <thead>
                <tr><th>Field group</th><th>Guidance</th></tr>
              </thead>
              <tbody>
                {agencyRows.map(([field, guidance]) => (
                  <tr key={field}><td>{field}</td><td>{guidance}</td></tr>
                ))}
              </tbody>
            </table>
            <div className="manual-callout manual-warning">
              RC Series is required for agency-template scans. If it is blank, Start Scan stays disabled.
            </div>
          </section>

          <section id="manual-email-copy" className="manual-section">
            <p className="manual-kicker">Workflow</p>
            <h3>Email Copy</h3>
            <p>
              Use Email Copy when you need to gather email files from a larger folder tree. The app copies recognized
              email files into the destination folder, keeps their folder layout, and writes a manifest listing what was
              copied.
            </p>
            <ul className="manual-chip-list" aria-label="Recognized email extensions">
              {emailTypes.map(type => <li key={type}><code>{type}</code></li>)}
            </ul>
          </section>

          <section id="manual-clone-compare" className="manual-section">
            <p className="manual-kicker">Workflow</p>
            <h3>Clone Compare</h3>
            <p>
              Use Clone Compare to check whether two drives or folders contain the same files. Choose the same
              verification setting for both sides. Single drive mode lets you scan Drive A, swap media, then scan Drive B.
            </p>
            <dl className="manual-definitions">
              <div><dt>Exact Clone</dt><dd>Everything checked matched.</dd></div>
              <div><dt>Content Clone</dt><dd>The files appear to match, but folder layout or saved details differ.</dd></div>
              <div><dt>Metadata Clone</dt><dd>The app found likely PDF saved-detail differences only.</dd></div>
              <div><dt>Not a Clone</dt><dd>Files are missing, extra, or different.</dd></div>
            </dl>
          </section>

          <section id="manual-outputs" className="manual-section">
            <p className="manual-kicker">Reference</p>
            <h3>Outputs</h3>
            <ul className="manual-list">
              <li>The app names output files with the source folder name and the date and time of the job.</li>
              <li>CSV files open in Excel and other spreadsheet tools.</li>
              <li>XLSX files are Excel workbooks. Create one when the final handoff should be an Excel file.</li>
              <li>The report file includes totals, the options used, skipped-file examples, and file type summaries.</li>
              <li>Very large jobs may be split into multiple spreadsheet parts so Excel can still open them.</li>
            </ul>
          </section>

          <section id="manual-updates" className="manual-section">
            <p className="manual-kicker">Reference</p>
            <h3>Updates</h3>
            <p>
              On Windows, the app checks the <code>X:\Apps</code> folder for a newer signed <code>content-list-generator.exe</code>.
              Open About if support gives you a different release folder.
            </p>
            <ul className="manual-list">
              <li>The app only installs a newer signed executable from the configured release folder.</li>
              <li>When an update is ready, restart the app from the update prompt.</li>
            </ul>
          </section>

          <section id="manual-troubleshooting" className="manual-section">
            <p className="manual-kicker">Support</p>
            <h3>Troubleshooting</h3>
            <div className="manual-support-grid">
              <div>
                <h4>macOS security prompt</h4>
                <p>Right-click the app, choose Open, then confirm. You usually only need to do this the first time.</p>
              </div>
              <div>
                <h4>Windows SmartScreen</h4>
                <p>Choose More info, then Run anyway if this app came from your trusted release location.</p>
              </div>
              <div>
                <h4>Slow scans</h4>
                <p>Network drives, very large files, and verification all add time. Choose Off for verification when you only need a quick list.</p>
              </div>
              <div>
                <h4>Reset settings</h4>
                <p>Open About and share the settings location with support if the app needs to be reset.</p>
              </div>
            </div>
          </section>
        </article>
      </div>
    </div>
  )
}
