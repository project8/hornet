Hornet
======

Hornet is the automatic data-flow software package for the Project 8 experiment.


Organization
------------

The various functions of hornet are performed by a number of semi-independent modules:

* :doc:`Watcher <watcher>` -- monitors a portion of the filesystem for new files to process;

* :doc:`Classifier <classifier>` -- recognizes files of certain types and how to proces them;

* :doc:`Mover <mover>` -- copies files from the "hot" data storage to the "warm" data storage;

* Worker -- performs nearline analysis on all or some files;

* :doc:`Shipper <shipper>` -- move files to a "cold" storage file system;

* :doc:`Scheduler <scheduler>` -- coordinate the processing of the files with the hornet modules.


Operation and Configuration
---------------------------

For obtaining and building hornet, please see the README file.

The hornet executable is, appropriately, called ``hornet``.  It is configured with a JSON configuration file that is supplied at runtime::

  > hornet --config my_config.json

You can find an example configuration file that includes settings for all of the available options in the ``examples`` directory.

The links in the Organization section will provide details on how to configure each module.  File hashing is performed in multiple places, and has its own configuration as well.


Repository
----------

The directory structure of the ``hornet`` repository is as follows:

* The README, license, source for the ``hornet`` executable, and the makefile for processing the git repository information exist in the top-level directory;
* ``documentation`` includes the documentation you're currently reading;
* ``examples`` includes the full example configuration file;
* ``gogitver`` contains the source for the external package ``gogitver``, which is used make hornet aware of the git repository information;
* ``hornet`` contains the source files for the ``hornet`` library, including all of the modules.
