Hornet Concepts
===============

Directory Structure
-------------------

In Hornet a file's location is considered to consist of a base directory, optionally followed by some set of subdirectories.  When a file moves between the hot storage, the warm storage, and the cold storage, the base directory will change, but the subdirectories will remain the same.

When Hornet is configured there are a special set of directories that will be considered base directories.  If a file is not located in one of those directories, then its entire directory path is its base directory.  The list of special base directories includes the following (in order):

1. The singular watch directory specified in the configuration as ``watcher.dir``.
2. The multiple watch directories specified in the configuration as ``watcher.dirs``.
3. The base directories specified in the configuration as ``classifier.base-paths``.

For a given file, the process of determining its base directory involves checking each element in the list of base directories, in the order above, until the first match is found; if no match is found, the full directory path is the base directory.  A match is defined as the absolute path for the file starting with the given directory path.

Examples
""""""""

Assume that the following directories are specified in a Hornet configuration file::

    watcher.dir = "/new-data"
    classifier.base-paths = ["/my/data1", /my/data2"]
    mover.dest-dir = "/bigdisk/all-data"
    shipper.dest-dir = "/coldstorage"

The following examples should cover most situations:

1) The user submits the file ``/some-other-disk/good-data/day0/run123.egg``.  The base path is ``/some-other-disk/good-data/day0`` because none of the special base directories match the directory path.  The resulting file locations are::

	* Warm path: ``/bigdisk/all-data/run123.egg``
	* Cold path: ``/coldstorage/run123.egg``

2) The user submits the file ``/my/data1/run234.egg``.  The base path is ``/my/data1`` because that is one of the special directories.  The resulting file locations are:
	* Warm path: ``/bigdisk/all-data/run234.egg``
	* Cold path: ``/coldstorage/run234.egg``

3) The user submits the file ``/my/data2/day391/run345.egg``. The base path is ``/my/data2`` because that is one of the special directories.  The resulting file locations are:

	* Warm path: ``/bigdisk/all-data/day391/run345.egg``
	* Cold path: ``/coldstorage/day391/run345.egg``

4) After starting Hornet, the file ``/new-data/run456.egg`` is created.  The base path is ``/new-data`` because that is one of the special directories.  The resulting file locations are:

	* Warm path: ``/bigdisk/all-data/run456.egg``
	* Cold path: ``/coldstorage/run456.egg``

5) After starting Hornet, the file ``/new-data/month4/week2/day5/run567.egg`` is created.  The base path is ``/new-data`` because that is one of the special directories. The resulting file locations are:

	* Warm path: ``/bigdisk/all-data/month4/week2/day5/run567.egg``
	* Cold path: ``/coldstorage/month4/week2/day5/run567.egg``


File Header Information
-----------------------

Each file is accompanied in Hornet by information that, primarily, describes where the file was/is/will be located.  It also contains information about the jobs that are to be performed and those that have already finished.

* Filename \* -- without path information.
* FileType \*\* -- as recognized by the :doc:`Classifier <classifier>`
* FileHash \*\* -- if calculated
* SubPath \*\* -- the subdirectory path (see above)
* HotPath \* -- the absolute directory path in hot storage
* WarmPath \*\*\* -- the absolute directory path in warm storage
* ColdPath \*\*\*\* -- the directory path in cold storage (absolute if local; may not be absolute if remote)
* FileHotPath \* -- the absolute file path in hot storage
* FileWarmPath \*\*\* -- the absolute file path in warm storage
* FileColdPath \*\*\*\* -- the file path in cold storage (absolute if local; may not be absolute if remote)
* JobQueue \*\* -- the jobs that will be performed (empty after processing by a Worker)
* FinishedJobs -- the jobs that have been completed

\* **(dev)** Filled in by the Scheduler

\*\* **(dev)** Filled in by the Classifier

\*\*\* **(dev)** Filled in by the Mover

\*\*\*\* **(dev)** Filled in by the Shipper