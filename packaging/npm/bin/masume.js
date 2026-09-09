#!/usr/bin/env node
const { spawnSync } = require("node:child_process");
const { createHash } = require("node:crypto");
const fs = require("node:fs");
const { tmpdir } = require("node:os");
const path = require("node:path");

const packageDirectory = path.join(__dirname, "..");
const { version, goBinary } = require("../package.json");
const binaryPath = path.join(__dirname, goBinary.name);

const operatingSystems = { darwin: "darwin", linux: "linux" };
const architectures = { x64: "amd64", arm64: "arm64" };

function buildDownloadUrl() {
	const operatingSystem = operatingSystems[process.platform];
	const architecture = architectures[process.arch];
	if (!operatingSystem || !architecture) {
		throw new Error(`masume has no build for ${process.platform} ${process.arch}`);
	}
	return goBinary.url
		.replace(/{{version}}/g, version)
		.replace(/{{platform}}/g, operatingSystem)
		.replace(/{{arch}}/g, architecture);
}

function findExpectedChecksum(archiveName) {
	const checksumsPath = path.join(packageDirectory, "checksums.txt");
	if (!fs.existsSync(checksumsPath)) {
		throw new Error("checksums.txt is missing from the package");
	}
	for (const line of fs.readFileSync(checksumsPath, "utf8").split("\n")) {
		const [checksum, name] = line.trim().split(/\s+/);
		if (name === archiveName) {
			return checksum;
		}
	}
	throw new Error(`${archiveName} is not listed in checksums.txt`);
}

async function downloadArchive(url, expectedChecksum) {
	const response = await fetch(url, { redirect: "follow" });
	if (!response.ok) {
		throw new Error(`${url} returned ${response.status} ${response.statusText}`);
	}
	const archive = Buffer.from(await response.arrayBuffer());
	const checksum = createHash("sha256").update(archive).digest("hex");
	if (checksum !== expectedChecksum) {
		throw new Error(`the download does not match its checksum; expected ${expectedChecksum}, got ${checksum}`);
	}
	return archive;
}

function extractBinary(directory, archiveName) {
	const extraction = spawnSync("tar", ["-xzf", archiveName, goBinary.name], {
		cwd: directory,
		stdio: "inherit",
	});
	if (extraction.error) {
		throw new Error(`tar cannot run: ${extraction.error.message}`);
	}
	if (extraction.status !== 0) {
		throw new Error(`${archiveName} holds no ${goBinary.name} binary`);
	}
	fs.copyFileSync(path.join(directory, goBinary.name), `${binaryPath}.download`);
	fs.chmodSync(`${binaryPath}.download`, 0o755);
	fs.renameSync(`${binaryPath}.download`, binaryPath);
}

async function downloadBinary() {
	const url = buildDownloadUrl();
	const archiveName = path.basename(new URL(url).pathname);

	process.stderr.write(`downloading ${url}\n`);
	const archive = await downloadArchive(url, findExpectedChecksum(archiveName));

	const directory = fs.mkdtempSync(path.join(tmpdir(), "masume-"));
	try {
		fs.writeFileSync(path.join(directory, archiveName), archive);
		extractBinary(directory, archiveName);
	} finally {
		fs.rmSync(directory, { recursive: true, force: true });
	}
}

async function installBinary() {
	if (!fs.existsSync(binaryPath)) {
		await downloadBinary();
	}
}

async function runBinary() {
	await installBinary();
	const run = spawnSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
	if (run.error) {
		throw new Error(`${binaryPath} cannot run: ${run.error.message}`);
	}
	process.exit(run.status === null ? 1 : run.status);
}

const start = process.env.npm_lifecycle_event === "postinstall" ? installBinary : runBinary;

start().catch((error) => {
	process.stderr.write(`masume: ${error.message}\n`);
	process.exit(1);
});
