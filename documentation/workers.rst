Workers
=======

The Workers are responsible for performing nearline analysis "jobs" on the files processed by Hornet.


Configuration
-------------

**(dev)** Note that all of these configuration values are processed by the Classifier, but pertain specifically to the workers.

::

    "workers":
    {
        "n-workers": 5,
        "jobs":
        [
            {
                "name": "proc-egg",
                "file-type": "egg",
                "command": "echo \"here's an egg file: {{.Filename}}\""
            },
            {
                "name": "proc-rsa-mat",
                "file-type": "rsa-mat",
                "command": "echo \"here's an RSA MAT file {{.Filename}}\""
            }
        ]
    }

* ``n-workers`` (unsigned integer): specifies the number of workers available to process files.
* ``jobs`` (array): lists the jobs that are performed for each file type.
* ``[job].name`` (string): unique identifier for each job type.
* ``[job].file-type`` (string): the file type that this job should be applied to. See :doc:`Classifier <classifier>` for information about file types.
* ``[job].command`` (string): the command that will be run to execute this job (see below).


Job Commands
------------

The job command should be a command that can be run on the command line.  Quotation marks can be escaped with `\`.

Hornet also supports limited variable substitution.  The syntax is demonstrated in the Configuration section above.  Before a job is processed, ``{{.Filename}}`` will be replaced by the value of the ``Filename`` variable.  The following variables are available:

* ``Filename``: the filename (no directory path included)
* ``FileType``: the file type, as identified by the :doc:`Classifier <classifier>`
* ``FileHash``: the file hash, if it was calculated (see the :doc:`Classifier <classifier>`)
* ``SubPath``: the subdirectory path (see the Directory Structure section of :doc:`Concepts <concepts>`)
* ``HotPath``: the absolute directory of the file in hot storage
* ``WarmPath``: the absolute directory of the file in warm storage
* ``ColdPath``: the directory of the file in cold storage (absolute if local; may not be absolute if remote) 
* ``FileHotPath``: the absolute path of the file in hot storage
* ``FileWarmPath``: the absolute path of the file in warm storage
* ``FileColdPath``: the path of the file in cold storage (absolute if local; may not be absolute if remote)


Chained Jobs the Reliable Way
-----------------------------

Once a Worker has started working on a particular file, all of the jobs requested for that file will be performed.  For example, if one job is analyzing an egg file and producing a ROOT file, you can specify a second job that analyzes the ROOT file for some sort of meta-analysis as long as you can specify the second job's command based on the original file's information.


Chained Jobs the Possibly-Not-So-Reliable Way
---------------------------------------------

Jobs can be chained together by configuring Hornet to watch for and recognize the output files from one type of job, and use them as input files.

In the current version of Hornet (2.0.0) this feature is only patially useful, depending on the workers' workload.  If all of the Workers are busy, files will bypass the Worker stage. Let's say that you are processing raw egg files and producing ROOT files at one stage of the analysis, and you would like a second stage that processes the ROOT files produced in the first stage.   If the Workers' load is limiting the number of files that actually get analyzed, then only some of the ROOT files that are produced will be analyzed.  Hopefully this situation will be improved in the future.